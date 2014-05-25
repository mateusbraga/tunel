package main

import (
	"crypto/tls"
	"github.com/mateusbraga/tunel/pkg/tunnel"
	"net/rpc"
	"sync"
)

var (
	serverConnTable   = make(map[string]*rpc.Client)
	serverConnTableMu sync.Mutex
)

func getOrCreateServerConn(addr string) (*rpc.Client, error) {
	serverConnTableMu.Lock()
	defer serverConnTableMu.Unlock()

	rpcClient, ok := serverConnTable[addr]
	if !ok {
		tlsConn, err := tls.Dial("tcp", addr, clientTlsConfig)
		if err != nil {
			return nil, err
		}
		rpcClient = rpc.NewClient(tlsConn)

		serverConnTable[addr] = rpcClient
	}
	return rpcClient, nil
}

type SendMsg struct {
	ConnId
	MsgNumber uint64
	Data      []byte
}

type ConnId struct {
	tunnel.Tunnel
	ConnNumber int
}

type tunnelConn struct {
	serviceMethod string
	ConnId
	*rpc.Client
	lastSentMsgNumber *uint64
	resultChan        chan *rpc.Call
	closeChan         chan struct{}
	err               error
	mutex             sync.Mutex
}

func NewTunnelConn(serviceMethod string, connId ConnId, rpcClient *rpc.Client) tunnelConn {
	var lastNumber uint64
	tnnlConn := tunnelConn{serviceMethod: serviceMethod, ConnId: connId, Client: rpcClient, lastSentMsgNumber: &lastNumber, resultChan: make(chan *rpc.Call, 16), closeChan: make(chan struct{})}

	go tunnelConnWorker(tnnlConn)

	return tnnlConn
}

func tunnelConnWorker(tnnlConn tunnelConn) {
	for {
		select {
		case _ = <-tnnlConn.closeChan:
			return
		case call := <-tnnlConn.resultChan:
			if call.Error != nil {
				tnnlConn.mutex.Lock()
				tnnlConn.err = call.Error
				tnnlConn.mutex.Unlock()

				tnnlConn.Close()
			}
		}
	}
}

func (t tunnelConn) Write(data []byte) (int, error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if t.err != nil {
		return 0, t.err
	}

	*t.lastSentMsgNumber++
	msg := SendMsg{ConnId: t.ConnId, Data: data, MsgNumber: *t.lastSentMsgNumber}
	t.Client.Go(t.serviceMethod, msg, &struct{}{}, t.resultChan)
	return len(data), nil
}

func (t tunnelConn) Close() error {
	select {
	case _ = <-t.closeChan:
		//already closed
	default:
		close(t.closeChan)
	}
	return nil
}
