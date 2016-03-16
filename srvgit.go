package main

import (
	"log"
	"net/http"
)

func main() {
	log.Fatal(http.ListenAndServe(":8079", http.FileServer(http.Dir("."))))
}
