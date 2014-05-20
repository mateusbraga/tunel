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

var (
	rootCert   = flag.String("rootcert", "rootcert.pem", "PEM file with trusted certificates to be used by this client to verify server certificates")
	clientKeys = flag.String("clientkeys", "server.pem", "PEM file with certificate chain to be used by this client")

	tlsConfig = new(tls.Config)
)

func main() {
	flag.Parse()
	src := flag.Arg(0)
	srcServer := flag.Arg(1)
	dstServer := flag.Arg(2)
	dst := flag.Arg(3)

	tnnl := tunnel.New(src, srcServer, dstServer, dst)

	tlsConn, err := tls.Dial("tcp", tnnl.SrcServer, tlsConfig)
	if err != nil {
		log.Panicf("Failed to dial SrcServer %v: %v\n", tnnl.SrcServer, err)
	}
	rpcClient := rpc.NewClient(tlsConn)

	err = rpcClient.Call("SrcServerService.SetUpSrcTunnel", tnnl, new(struct{}))
	if err != nil {
		log.Fatalln("Fatal SrcServer call:", err)
	}

	log.Println("Tunnel created.")
}

func init() {
	cert, err := tls.LoadX509KeyPair(*clientKeys, *clientKeys)
	if err != nil {
		log.Panicln(err)
	}

	tlsConfig.Certificates = append(tlsConfig.Certificates, cert)

	rootCertBytes, err := ioutil.ReadFile(*rootCert)
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
