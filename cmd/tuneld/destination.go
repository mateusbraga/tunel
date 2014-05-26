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

// serveDstConn starts once every DstServerService.Dial and forwards data received from Dst
// to SrcServer.
func serveDstConn(dst *dstConn) {
	defer stopDstConn(dst)

	rpcClient, err := getOrCreateServerConn(dst.SrcServer)
	if err != nil {
		log.Printf("Failed to connect to source server %v: %v\n", dst.SrcServer, err)
		return
	}

	tnnlConn := NewTunnelConn("SrcServerService.Receive", dst.ConnId, rpcClient)
	defer tnnlConn.Close()

	// keep sending data back from Dst to SrcServer (which will then go to Src)
	sent, err := io.Copy(tnnlConn, dst.Conn)
	if err != nil {
		log.Printf("Dst-side connection %v broke down: %v\n", dst.ConnId, err)
		return
	}

	log.Printf("Dst-side connection ended %v. %v bytes was sent to Src in total.", dst.ConnId, sent)
}

func stopDstConn(dst *dstConn) {
	dstConnTableMu.Lock()
	defer dstConnTableMu.Unlock()

	delete(dstConnTable, dst.ConnId)

	err := dst.Conn.Close()
	if err != nil {
		log.Printf("Error while closing Dst connection %v: %v\n", dst.ConnId, err)
	}

	rpcClient, err := getOrCreateServerConn(dst.SrcServer)
	if err != nil {
		log.Printf("Error while asking to close Src connection %v: %v\n", dst.ConnId, err)
		return
	}
	err = rpcClient.Call("SrcServerService.CloseSrcConn", dst.ConnId, new(struct{}))
	if err != nil {
		log.Printf("Error while asking to close Src connection %v: %v\n", dst.ConnId, err)
	}
}

type dstConn struct {
	ConnId
	net.Conn
	lastSeenMsgNumber uint64
	*sync.Cond
}

type DstServerService struct{}

func init() { rpc.Register(new(DstServerService)) }

// Send is called by SrcServer to pass data to DstServer
func (s *DstServerService) Send(msg SendMsg, nop *struct{}) error {
	dstConnTableMu.Lock()
	dst, ok := dstConnTable[msg.ConnId]
	if !ok {
		dstConnTableMu.Unlock()
		return fmt.Errorf("Connection is not up")
	}
	dstConnTableMu.Unlock()

	log.Println("Got ", msg.MsgNumber, msg.ConnId)

	dst.L.Lock()
	defer dst.L.Unlock()
	for msg.MsgNumber != dst.lastSeenMsgNumber+1 {
		log.Printf("Got %v, last seen %v\n", msg.MsgNumber, dst.lastSeenMsgNumber)
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
	log.Printf("Sent %v bytes to %v (%v) MsgNumber %v\n", len(msg.Data), msg.Tunnel.Dst, msg.ConnId, msg.MsgNumber)
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

	dst := &dstConn{ConnId: *connId, Conn: conn, lastSeenMsgNumber: 0, Cond: sync.NewCond(new(sync.Mutex))}

	dstConnTable[*connId] = dst
	go serveDstConn(dst)

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
	log.Println("Src-side closed Dst-side connection", connId)
	return nil
}
