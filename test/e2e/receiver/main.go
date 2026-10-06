package main

import (
	"log"
	"net/http"
	"time"
)

func main() {
	server := NewServer()
	httpServer := &http.Server{
		Addr:              ":8080",
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(httpServer.ListenAndServe())
}
