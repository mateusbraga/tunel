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
	rootCert   = flag.String("rootcert", "rootcert.pem", "PEM file with trusted certificates to be used by this server")
	serverKeys = flag.String("serverkeys", "server.pem", "PEM file with certificate chain to be used by this server")

	serverTlsConfig = new(tls.Config)
	clientTlsConfig = new(tls.Config)
)

func main() {
	flag.Parse()

	listener, err := tls.Listen("tcp", *thisServer, serverTlsConfig)
	if err != nil {
		log.Fatalf("Failed to setup listener at %v: %v\n", *thisServer, err)
	}
	log.Println("Listening on address:", listener.Addr())

	// Uncomment this to see tls error
	//c, err := listener.Accept()
	//if err != nil {
	//log.Println(err)
	//}

	//b := make([]byte, 100)
	//_, err = c.Read(b)
	//if err != nil {
	//log.Println(err)
	//}

	rpc.Accept(listener)
}

func init() {
	cert, err := tls.LoadX509KeyPair(*serverKeys, *serverKeys)
	if err != nil {
		log.Panicln(err)
	}

	serverTlsConfig.Certificates = append(serverTlsConfig.Certificates, cert)

	rootCertBytes, err := ioutil.ReadFile(*rootCert)
	if err != nil {
		log.Panicln(err)
	}

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(rootCertBytes)
	if !ok {
		log.Panicln("Failed to init root certificate")
	}
	serverTlsConfig.ClientCAs = certPool

	serverTlsConfig.ClientAuth = tls.RequireAndVerifyClientCert

	//TODO remove this for real security
	serverTlsConfig.InsecureSkipVerify = true
}

func init() {
	cert, err := tls.LoadX509KeyPair(*serverKeys, *serverKeys)
	if err != nil {
		log.Panicln(err)
	}

	clientTlsConfig.Certificates = append(clientTlsConfig.Certificates, cert)

	rootCertBytes, err := ioutil.ReadFile(*rootCert)
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
