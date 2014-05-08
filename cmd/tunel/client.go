package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"github.com/mateusbraga/tunel/pkg/tunnel"
	"io/ioutil"
	"log"
	"net/rpc"
)

var tlsConfig = new(tls.Config)

func main() {
	flag.Parse()
	src := flag.Arg(0)
	srcServer := flag.Arg(1)
	dstServer := flag.Arg(2)
	dst := flag.Arg(3)

	tnnl := tunnel.New(src, srcServer, dstServer, dst)

	tlsConn, err := tls.Dial("tcp", tnnl.SrcServer, tlsConfig)
	if err != nil {
		log.Panicln("Failed to dial SrcServer:", err)
	}
	rpcClient := rpc.NewClient(tlsConn)

	err = rpcClient.Call("SrcServerService.SetUpSrcTunnel", tnnl, new(struct{}))
	if err != nil {
		log.Fatalln("Fatal SrcServer call:", err)
	}

	log.Println("Tunnel created.")
}

func init() {
	cert, err := tls.LoadX509KeyPair("server.pem", "server.pem")
	if err != nil {
		log.Panicln(err)
	}

	tlsConfig.Certificates = append(tlsConfig.Certificates, cert)

	rootCertBytes, err := ioutil.ReadFile("rootcert.pem")
	if err != nil {
		log.Panicln(err)
	}

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(rootCertBytes)
	if !ok {
		log.Panicln("Failed to init root certificate")
	}
	tlsConfig.RootCAs = certPool
}
