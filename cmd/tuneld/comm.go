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
	lastAckMsgNumber  *uint64
	resultChan        chan *rpc.Call
	closing           bool
	closeChan         chan struct{}
	err               error
	mutex             sync.Mutex
}

func NewTunnelConn(serviceMethod string, connId ConnId, rpcClient *rpc.Client) tunnelConn {
	var lastNumber uint64
	var lastAck uint64
	tnnlConn := tunnelConn{serviceMethod: serviceMethod, ConnId: connId, Client: rpcClient, lastSentMsgNumber: &lastNumber, lastAckMsgNumber: &lastAck, resultChan: make(chan *rpc.Call, 16), closeChan: make(chan struct{})}

	go tunnelConnWorker(tnnlConn)

	return tnnlConn
}

func tunnelConnWorker(t tunnelConn) {
	for {
		call := <-t.resultChan
		if call.Error != nil {
			t.mutex.Lock()
			t.err = call.Error
			t.mutex.Unlock()

			close(t.closeChan)
			return
		}

		t.mutex.Lock()
		lastAckMsgNumber := call.Reply.(*uint64)
		if *lastAckMsgNumber > *t.lastAckMsgNumber {
			*t.lastAckMsgNumber = *lastAckMsgNumber
		}
		if t.closing && *t.lastSentMsgNumber == *t.lastAckMsgNumber {
			t.mutex.Unlock()
			close(t.closeChan)
			return
		}
		t.mutex.Unlock()
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
	var lastAckMsgNumber uint64
	t.Client.Go(t.serviceMethod, msg, &lastAckMsgNumber, t.resultChan)
	return len(data), nil
}

func (t tunnelConn) Close() error {
	t.mutex.Lock()
	t.closing = true
	t.mutex.Unlock()

	<-t.closeChan
	return nil
}
