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
	tunnelEntranceTable   = make(map[tunnel.Tunnel]*tunnelEntrance)
	tunnelEntranceTableMu sync.Mutex

	srcConnTable   = make(map[ConnId]net.Conn)
	srcConnTableMu sync.Mutex
)

type tunnelEntrance struct {
	mutex sync.RWMutex
	tunnel.Tunnel
	net.Listener
	closing bool
}

// TODO clean up srcWorkersTable
//defer func() {
//srcWorkersTableMu.Lock()
//delete(srcWorkersTable, worker.Tunnel)
//srcWorkersTableMu.Unlock()
//}()
func (t *tunnelEntrance) Accept() {
	defer t.done()

	listener, err := net.Listen("tcp", t.Src)
	if err != nil {
		log.Printf("Failed to setup listener at %v: %v\n", t.Src, err)
		return
	}
	t.mutex.Lock()
	t.Listener = listener
	t.mutex.Unlock()

	// start connection with DstServer to speed up first connection by doing ssl handshake now
	_, err = getOrCreateServerConn(t.DstServer)
	if err != nil {
		log.Printf("Failed to connect to destination server %v: %v\n", t.DstServer, err)
		return
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			t.mutex.RLock()
			if t.closing {
				t.mutex.RUnlock()
				return
			}
			t.mutex.RUnlock()

			log.Printf("Listener at %v failed to accept new connection: %v\n", t.Tunnel.Src, err)
			return
		}

		go t.ServeConn(conn)
	}
}

func (t *tunnelEntrance) ServeConn(conn net.Conn) {
	defer func() {
		err := conn.Close()
		if err != nil {
			log.Printf("Failed to close connection %v: %v\n", conn.LocalAddr(), err)
		}
	}()

	// connect to DstServer
	rpcClient, err := getOrCreateServerConn(t.DstServer)
	if err != nil {
		log.Printf("Failed to connect to destination server %v: %v\n", t.DstServer, err)
		return
	}

	// open connection with Dst
	var connId ConnId
	err = rpcClient.Call("DstServerService.Dial", t.Tunnel, &connId)
	if err != nil {
		log.Printf("Failed to complete connection through tunnel with destination server %v: %v\n", t.DstServer, err)
		return
	}
	srcConnTableMu.Lock()
	srcConnTable[connId] = conn
	srcConnTableMu.Unlock()
	defer func() {
		err = rpcClient.Call("DstServerService.CloseDstConn", connId, new(struct{}))
		if err != nil {
			log.Printf("Error while asking to close Dst connection %v: %v\n", connId, err)
		}
	}()

	tnnlConn := NewTunnelConn("DstServerService.Send", connId, rpcClient)
	defer tnnlConn.Close()

	// keep sending data from conn to DstServer (which will then go to Dst)
	sent, err := io.Copy(tnnlConn, conn)
	if err != nil {
		log.Printf("Src-side connection %v broke down: %v\n", connId, err)
		return
	}
	log.Printf("Src-side connection ended %v. %v bytes was sent to Dst in total.", connId, sent)
}

func (t *tunnelEntrance) done() {
}

type srcWorker struct {
	tunnel.Tunnel
	net.Listener
}

type SrcServerService struct{}

func init() { rpc.Register(new(SrcServerService)) }

func (s *SrcServerService) SetUpSrcTunnel(tnnl tunnel.Tunnel, ack *struct{}) error {
	tunnelEntranceTableMu.Lock()
	defer tunnelEntranceTableMu.Unlock()
	_, ok := tunnelEntranceTable[tnnl]
	if ok {
		return fmt.Errorf("Tunnel already exists.")
	}

	t := &tunnelEntrance{}
	t.Tunnel = tnnl

	tunnelEntranceTable[tnnl] = t

	go t.Accept()

	log.Println("New tunnel is up:", tnnl)
	return nil
}

// Receive is called by DstServer to pass data to SrcServer
func (s *SrcServerService) Receive(msg SendMsg, nop *struct{}) error {
	srcConnTableMu.Lock()
	defer srcConnTableMu.Unlock()

	conn, ok := srcConnTable[msg.ConnId]
	if !ok {
		return fmt.Errorf("Connection is not up")
	}

	sent, err := conn.Write(msg.Data)
	if err != nil {
		return err
	}
	if sent != len(msg.Data) {
		return fmt.Errorf("Expected to send %d bytes, but sent only %d", len(msg.Data), sent)
	}
	log.Printf("Sent %v bytes back to %v (%v)\n", len(msg.Data), msg.Tunnel.Src, msg.ConnId)
	return nil
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
	log.Println("Dst-side closed Src-side connection", connId)
	return nil
}
