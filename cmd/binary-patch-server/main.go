package main

import (
	"io"
	"log"
	"net/http"
	"os"
)

type PatchServer struct{}

var filename = "./binary-patch"

func (ps PatchServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	fd, err := os.Open(filename)
	if err != nil {
		log.Fatalf("Can not open file caused by: %v", err)
	}
	defer fd.Close()
	if _, err := io.Copy(w, fd); err != nil {
		log.Fatalf("Could not copy %s to client, caused by: %v", filename, err)
	}
}

func main() {
	var ps PatchServer
	err := http.ListenAndServe("localhost:8080", ps)
	if err != nil {
		log.Fatal(err)
	}
}
