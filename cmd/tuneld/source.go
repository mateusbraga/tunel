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
	srcWorkersTable   = make(map[tunnel.Tunnel]*srcWorker)
	srcWorkersTableMu sync.Mutex

	srcConnTable   = make(map[ConnId]net.Conn)
	srcConnTableMu sync.Mutex
)

func runSrcWorker(worker *srcWorker) {
	defer stopSrcWorker(worker)

	listener, err := net.Listen("tcp", worker.Src)
	if err != nil {
		log.Println("Failed to setup listener at %v: %v\n", worker.Src, err)
		return
	}
	worker.Listener = listener

	// start connection with DstServer
	_, err = getOrCreateServerConn(worker.DstServer)
	if err != nil {
		log.Println("Failed to connect to destination server %v: %v\n", worker.DstServer, err)
		return
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Listener at %v failed to accept new connection: %v\n", worker.Tunnel.DstServer, err)
			return
		}

		go handleSrcConn(worker, conn)
	}
}

func handleSrcConn(worker *srcWorker, conn net.Conn) {
	defer func() {
		err := conn.Close()
		if err != nil {
			log.Printf("Failed to close connection with %v: %v\n", conn.RemoteAddr(), err)
		}
	}()

	// connect to DstServer
	rpcClient, err := getOrCreateServerConn(worker.DstServer)
	if err != nil {
		log.Println("Failed to connect to destination server %v: %v\n", worker.DstServer, err)
		return
	}

	// open connection with Dst
	var connId ConnId
	err = rpcClient.Call("DstServerService.Dial", worker.Tunnel, &connId)
	if err != nil {
		log.Printf("Failed to complete connection through tunnel with destination server %v: %v\n", worker.DstServer, err)
		return
	}
	srcConnTableMu.Lock()
	srcConnTable[connId] = conn
	srcConnTableMu.Unlock()
	// Clean up connection
	defer func() {
		srcConnTableMu.Lock()
		delete(srcConnTable, connId)
		srcConnTableMu.Unlock()

		err = rpcClient.Call("DstServerService.CloseDstConn", connId, new(struct{}))
		if err != nil {
			log.Printf("Error while asking to close Dst connection %v: %v\n", connId, err)
		}
	}()

	tnnlConn := tunnelSendConn{ConnId: connId, Client: rpcClient}

	// keep sending data from conn to DstServer (which will then go to Dst)
	sent, err := io.Copy(tnnlConn, conn)
	if err != nil {
		log.Printf("Src-side connection %v broke down: %v\n", connId, err)
		return
	}
	log.Printf("Src-side connection %v ended. %v bytes was sent to Dst in total.", connId, sent)
}

func stopSrcWorker(worker *srcWorker) {
	srcWorkersTableMu.Lock()
	defer srcWorkersTableMu.Unlock()

	delete(srcWorkersTable, worker.Tunnel)
}

type srcWorker struct {
	tunnel.Tunnel
	net.Listener
}

type SrcServerService struct{}

func init() { rpc.Register(new(SrcServerService)) }

func (s *SrcServerService) SetUpSrcTunnel(tnnl tunnel.Tunnel, ack *struct{}) error {
	log.Println("New tunnel is up:", tnnl)
	srcWorkersTableMu.Lock()
	defer srcWorkersTableMu.Unlock()

	_, ok := srcWorkersTable[tnnl]
	if ok {
		return fmt.Errorf("Tunnel already exists.")
	}

	worker := new(srcWorker)
	worker.Tunnel = tnnl
	go runSrcWorker(worker)
	srcWorkersTable[tnnl] = worker
	return nil
}

// Receive is called by DstServer to pass data to SrcServer
func (s *SrcServerService) Receive(msg SendMsg, sent *int) error {
	srcConnTableMu.Lock()
	defer srcConnTableMu.Unlock()

	conn, ok := srcConnTable[msg.ConnId]
	if !ok {
		return fmt.Errorf("Connection is not up")
	}

	var err error
	*sent, err = conn.Write(msg.Data)
	log.Printf("Sent %v bytes back to %v (%v)\n", *sent, msg.Tunnel.Src, msg.ConnId)
	return err
}

func (s *SrcServerService) CloseSrcConn(connId ConnId, nothing *struct{}) error {
	srcConnTableMu.Lock()
	defer srcConnTableMu.Unlock()

	conn, ok := srcConnTable[connId]
	if !ok {
		// already closed
		return nil
	}

	err := conn.Close()
	if err != nil {
		return err
	}

	delete(srcConnTable, connId)
	log.Println("Forced close of Src-side connection", connId)
	return nil
}
