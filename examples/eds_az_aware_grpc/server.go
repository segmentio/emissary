package main

import (
	"net/http"
	"log"
	"fmt"
	"os"
)

var (
	serverNum string
)
func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, fmt.Sprintf("hello from %s", serverNum))
}


func main() {
	serverNum = os.Args[1]
	http.HandleFunc("/", handler)
	log.Fatal(http.ListenAndServe(":" + os.Args[2], nil))
}
