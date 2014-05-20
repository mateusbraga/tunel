package main

import (
	"flag"
	"fmt"
	"html"
	"log"
	"net/http"
)

var (
	bind = flag.String("bind", ":8080", "Address to listen to")
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, %q", html.EscapeString(r.URL.Path))
	})
	log.Println("Listening at:", *bind)
	log.Fatal(http.ListenAndServe(*bind, nil))
}
