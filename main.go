package main

import (
	"example.com/m/server"
	
	"net/http"

	"golang.org/x/net/websocket"
)

func main() {
	if server.ShouldLog {
		server.InitDB()
	}
	server := server.NewServer()
	http.Handle("/ws/", websocket.Handler(server.HandleWs))
	http.ListenAndServe(":3000", nil)
}
