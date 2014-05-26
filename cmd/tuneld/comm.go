package main

import (
	"crypto/tls"
	"fmt"
	"github.com/mateusbraga/tunel/pkg/tunnel"
	"log"
	"net"
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

type tunnelConnSender struct {
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

func NewTunnelConnSender(serviceMethod string, connId ConnId, rpcClient *rpc.Client) *tunnelConnSender {
	var lastNumber uint64
	var lastAck uint64
	tnnlConn := tunnelConnSender{serviceMethod: serviceMethod, ConnId: connId, Client: rpcClient, lastSentMsgNumber: &lastNumber, lastAckMsgNumber: &lastAck, resultChan: make(chan *rpc.Call, 16), closeChan: make(chan struct{})}

	go tunnelConnWorker(&tnnlConn)

	return &tnnlConn
}

func tunnelConnWorker(t *tunnelConnSender) {
	for {
		select {
		case call := <-t.resultChan:
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
		case <-t.closeChan:
			return
		}
	}
}

func (t tunnelConnSender) Write(data []byte) (int, error) {
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

func (t tunnelConnSender) Close() error {
	t.mutex.Lock()
	t.closing = true

	if *t.lastSentMsgNumber == *t.lastAckMsgNumber {
		close(t.closeChan)
	}
	t.mutex.Unlock()

	log.Println("Closing", t.ConnId)
	<-t.closeChan
	return nil
}

type tunnelConnReceiver struct {
	ConnId
	net.Conn
	lastSeenMsgNumber uint64
	msgMap            map[uint64]*SendMsg
	sync.Mutex
}

func (t *tunnelConnReceiver) fowardData(msg *SendMsg) (uint64, error) {
	t.Lock()
	defer t.Unlock()

	t.msgMap[msg.MsgNumber] = msg

	m, exist := t.msgMap[t.lastSeenMsgNumber+1]
	for exist {
		sent, err := t.Write(m.Data)
		if err != nil {
			return 0, err
		}
		if sent != len(m.Data) {
			return 0, fmt.Errorf("Expected to send %d bytes, but sent only %d", len(m.Data), sent)
		}
		log.Printf("Sent %v bytes to %v (%v) MsgNumber %v\n", len(m.Data), m.Tunnel.Dst, m.ConnId, m.MsgNumber)

		delete(t.msgMap, t.lastSeenMsgNumber+1)
		t.lastSeenMsgNumber++
		m, exist = t.msgMap[t.lastSeenMsgNumber+1]
	}
	return t.lastSeenMsgNumber, nil
}
