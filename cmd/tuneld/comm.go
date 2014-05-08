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
	Data []byte
}

type ConnId struct {
	tunnel.Tunnel
	ConnNumber int
}

type tunnelSendConn struct {
	ConnId
	*rpc.Client
}

func (t tunnelSendConn) Write(data []byte) (int, error) {
	msg := SendMsg{ConnId: t.ConnId, Data: data}
	var sent int
	err := t.Client.Call("DstServerService.Send", msg, &sent)
	return sent, err
}

type tunnelReceiveConn struct {
	ConnId
	*rpc.Client
}

func (t tunnelReceiveConn) Write(data []byte) (int, error) {
	msg := SendMsg{ConnId: t.ConnId, Data: data}
	var sent int
	err := t.Client.Call("SrcServerService.Receive", msg, &sent)
	return sent, err
}
