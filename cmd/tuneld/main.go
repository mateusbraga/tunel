package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"io/ioutil"
	"log"
	"net/rpc"
)

var (
	thisServer = flag.String("bind", ":4000", "Address to listen for")

	serverTlsConfig = new(tls.Config)
	clientTlsConfig = new(tls.Config)
)

func main() {
	listener, err := tls.Listen("tcp", *thisServer, serverTlsConfig)
	if err != nil {
		log.Fatalf("Failed to setup listener at %v: %v\n", *thisServer, err)
	}
	log.Println("Listening on address:", listener.Addr())

	rpc.Accept(listener)
}

func init() {
	cert, err := tls.LoadX509KeyPair("server.pem", "server.pem")
	if err != nil {
		log.Panicln(err)
	}

	serverTlsConfig.Certificates = append(serverTlsConfig.Certificates, cert)
}

func init() {
	cert, err := tls.LoadX509KeyPair("server.pem", "server.pem")
	if err != nil {
		log.Panicln(err)
	}

	clientTlsConfig.Certificates = append(clientTlsConfig.Certificates, cert)

	rootCertBytes, err := ioutil.ReadFile("rootcert.pem")
	if err != nil {
		log.Panicln(err)
	}

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(rootCertBytes)
	if !ok {
		log.Panicln("Failed to init root certificate")
	}
	clientTlsConfig.RootCAs = certPool
}
