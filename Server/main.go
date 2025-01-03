package main

import (
	"fmt"
	"net/http"
)

func main() {
	server := NewServer()

	http.HandleFunc("/chat", server.handleWebSocket)

	fmt.Println("Server started at Port 3000")
	http.ListenAndServe(":3000", nil)
}
