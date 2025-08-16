package main

import (
	"log"
	"net/http"
	"nostr-sample/relay"
)

func main() {
	srv := relay.NewServer()
	if err := srv.InitLogDir("log"); err != nil {
		log.Printf("log init error: %v", err)
	}
	http.Handle("/", srv.WithAccessLog(http.HandlerFunc(srv.HandleRelayInfo)))
	http.Handle("/ws", srv.WithAccessLog(http.HandlerFunc(srv.HandleWebSocket)))
	port := ":8080"
	log.Printf("Nostr relay server starting on port %s", port)
	log.Printf("WebSocket endpoint: ws://localhost%s/ws", port)
	log.Printf("Relay info: http://localhost%s/", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
