package main

import (
	"fmt"
	"net/http"
)

func main() {
	server := NewServer()

	http.HandleFunc("/chat", server.handleWebSocket)

	fmt.Println("Server started at Port 3002")
	http.ListenAndServe(":3002", nil)
}
