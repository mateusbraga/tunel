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
	dstConnTable   = make(map[ConnId]*tunnelConnReceiver)
	dstConnNextId  int
	dstConnTableMu sync.Mutex
)

// serveDstConn starts once every DstServerService.Dial and forwards data received from Dst
// to SrcServer.
func serveDstConn(dst *tunnelConnReceiver) {
	defer stopDstConn(dst)

	rpcClient, err := getOrCreateServerConn(dst.SrcServer)
	if err != nil {
		log.Printf("Failed to connect to source server %v: %v\n", dst.SrcServer, err)
		return
	}

	sender := NewTunnelConnSender("SrcServerService.Receive", dst.ConnId, rpcClient)
	defer sender.Close()

	// keep sending data back from Dst to SrcServer (which will then go to Src)
	sent, err := io.Copy(sender, dst.Conn)
	if err != nil {
		log.Printf("Dst-side connection %v broke down: %v\n", dst.ConnId, err)
		return
	}

	log.Printf("Dst-side connection ended %v. %v bytes in total was sent to Src.", dst.ConnId, sent)
}

func stopDstConn(dst *tunnelConnReceiver) {
	dstConnTableMu.Lock()
	defer dstConnTableMu.Unlock()

	delete(dstConnTable, dst.ConnId)

	err := dst.Conn.Close()
	if err != nil {
		//log.Printf("Error while closing Dst connection %v: %v\n", dst.ConnId, err)
	}

	rpcClient, err := getOrCreateServerConn(dst.SrcServer)
	if err != nil {
		log.Printf("Could not connect to SrcServer to ask to close Src connection %v: %v\n", dst.ConnId, err)
		return
	}
	err = rpcClient.Call("SrcServerService.CloseSrcConn", dst.ConnId, new(struct{}))
	if err != nil {
		log.Printf("Could not ask to close Src connection %v: %v\n", dst.ConnId, err)
	}
}

type DstServerService struct{}

func init() { rpc.Register(new(DstServerService)) }

// Send is called by SrcServer to pass data to DstServer
func (s *DstServerService) Send(msg SendMsg, lastMsgNumber *uint64) error {
	dstConnTableMu.Lock()
	dst, ok := dstConnTable[msg.ConnId]
	if !ok {
		dstConnTableMu.Unlock()
		return fmt.Errorf("Connection is not up")
	}
	dstConnTableMu.Unlock()

	log.Println("Got ", msg.MsgNumber, msg.ConnId)

	var err error
	*lastMsgNumber, err = dst.fowardData(&msg)
	return err
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

	dst := &tunnelConnReceiver{ConnId: *connId, Conn: conn, msgMap: make(map[uint64]*SendMsg, 64)}

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
