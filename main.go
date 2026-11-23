package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dir := flag.String("data", "data", "journal directory")
	flag.Parse()

	store, err := OpenStore(*dir)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	log.Printf("collabedit listening on %s, data dir %s", *addr, *dir)
	log.Fatal(http.ListenAndServe(*addr, NewServer(store)))
}
