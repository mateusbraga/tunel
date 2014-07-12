# tunel

Tunel implements a TLS tunneling program. It was developed as part of my studies in TLS and PKI.

# Development

tunel is written in [Go](http://golang.org). You'll need a recent version of Go installed on your computer to build it.

## Build

go build ./...

# Running

    # Start tunel servers
    tuneld -rootcert="rootcert.pem" -serverkeys="server.pem" -bind=":4000"
    tuneld -rootcert="rootcert.pem" -serverkeys="server2.pem" -bind=":4001"
    # Create tunel with client
    tunel :5050 :4000 :4001 :6060

    # use nc for testing; start server
    nc -l 127.0.0.1 6060
    # run client
    nc 127.0.0.1 5050

    # type anything and the server will receive it; the communication between :4000 and :4001 is encrypted.
