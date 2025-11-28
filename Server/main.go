package main

import (
	"fmt"
	"net/http"
)

func main() {
	server := NewServer()

	http.HandleFunc("/chat", server.handleWebSocket)

	fmt.Println("Server started at Port 3001")
	http.ListenAndServe(":3001", nil)
}
