package main

import (
	"fmt"
	"github.com/mateusbraga/tunel/pkg/tunnel"
	"io"
	"log"
	"net"
	"net/rpc"
	"sync"
)

var (
	dstConnTable   = make(map[ConnId]*dstConn)
	dstConnNextId  int
	dstConnTableMu sync.Mutex
)

type dstWorker struct {
	ConnId
	net.Conn
}

// runDstWorker starts once every DstServerService.Dial and forwards data received from Dst
// to SrcServer.
func runDstWorker(worker dstWorker) {
	defer stopDstWorker(worker)

	rpcClient, err := getOrCreateServerConn(worker.SrcServer)
	if err != nil {
		log.Println("Failed to connect to source server %v: %v\n", worker.SrcServer, err)
		return
	}

	tnnlConn := NewTunnelConn("SrcServerService.Receive", worker.ConnId, rpcClient)
	defer tnnlConn.Close()

	// keep sending data back from Dst to SrcServer (which will then go to Src)
	sent, err := io.Copy(tnnlConn, worker.Conn)
	if err != nil {
		log.Printf("Dst-side connection %v broke down: %v\n", worker.ConnId, err)
		return
	}

	log.Printf("Dst-side connection %v ended. %v bytes was sent to Src in total.", worker.ConnId, sent)
}

func stopDstWorker(worker dstWorker) {
	dstConnTableMu.Lock()
	defer dstConnTableMu.Unlock()

	delete(dstConnTable, worker.ConnId)

	err := worker.Conn.Close()
	if err != nil {
		log.Printf("Error while closing Dst connection %v: %v\n", worker.ConnId, err)
	}

	rpcClient, err := getOrCreateServerConn(worker.SrcServer)
	if err != nil {
		log.Printf("Error while asking to close Src connection %v: %v\n", worker.ConnId, err)
		return
	}
	err = rpcClient.Call("SrcServerService.CloseSrcConn", worker.ConnId, new(struct{}))
	if err != nil {
		log.Printf("Error while asking to close Src connection %v: %v\n", worker.ConnId, err)
	}
}

type dstConn struct {
	net.Conn
	lastSeenMsgNumber uint64
	*sync.Cond
}

type DstServerService struct{}

func init() { rpc.Register(new(DstServerService)) }

// Send is called by SrcServer to pass data to DstServer
func (s *DstServerService) Send(msg SendMsg, nop *struct{}) error {
	dstConnTableMu.Lock()
	defer dstConnTableMu.Unlock()

	dst, ok := dstConnTable[msg.ConnId]
	if !ok {
		return fmt.Errorf("Connection is not up")
	}

	dst.L.Lock()
	defer dst.L.Unlock()
	for msg.MsgNumber != dst.lastSeenMsgNumber+1 {
		dst.Wait()
	}

	sent, err := dst.Write(msg.Data)
	if err != nil {
		return err
	}
	if sent != len(msg.Data) {
		return fmt.Errorf("Expected to send %d bytes, but sent only %d", len(msg.Data), sent)
	}

	dst.lastSeenMsgNumber++
	dst.Broadcast()
	log.Printf("Sent %v bytes to %v (%v)\n", len(msg.Data), msg.Tunnel.Dst, msg.ConnId)
	return nil
}

// Dial open connection with Dst and starts a dstWorker.
func (s *DstServerService) Dial(tnnl tunnel.Tunnel, connId *ConnId) error {
	dstConnTableMu.Lock()
	defer dstConnTableMu.Unlock()

	// open connection with Dst
	conn, err := net.Dial("tcp", tnnl.Dst)
	if err != nil {
		return err
	}

	connId.Tunnel = tnnl
	connId.ConnNumber = dstConnNextId
	dstConnNextId++

	dst := &dstConn{Conn: conn, lastSeenMsgNumber: 0, Cond: sync.NewCond(new(sync.Mutex))}

	dstConnTable[*connId] = dst
	go runDstWorker(dstWorker{ConnId: *connId, Conn: conn})

	log.Printf("New connection with Dst %v is up: %v\n", tnnl.Dst, connId.ConnNumber)
	return nil
}

func (s *DstServerService) CloseDstConn(connId ConnId, nothing *struct{}) error {
	dstConnTableMu.Lock()
	defer dstConnTableMu.Unlock()

	conn, ok := dstConnTable[connId]
	if !ok {
		// already closed
		return nil
	}

	err := conn.Close()
	if err != nil {
		return err
	}

	delete(dstConnTable, connId)
	log.Println("Forced close of Dst-side connection", connId)
	return nil
}
