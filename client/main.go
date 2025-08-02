package main

import (
	"encoding/json"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nbd-wtf/go-nostr"
)

func main() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/ws"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("recv: %s", message)
		}
	}()

	// Subscribe to all events
	subMsg := []interface{}{"REQ", "sub1", map[string]interface{}{}}
	subData, _ := json.Marshal(subMsg)

	log.Printf("sending subscription: %s", subData)
	err = c.WriteMessage(websocket.TextMessage, subData)
	if err != nil {
		log.Println("write:", err)
		return
	}

	// Wait a bit and then send a test event
	time.Sleep(1 * time.Second)

	// Create a test event
	sk := nostr.GeneratePrivateKey()
	pub, _ := nostr.GetPublicKey(sk)

	event := nostr.Event{
		Kind:      1,
		CreatedAt: nostr.Now(),
		Content:   "Hello, Nostr! This is a test message from the Go client.",
		PubKey:    pub,
		Tags:      nostr.Tags{},
	}

	event.Sign(sk)

	eventMsg := []interface{}{"EVENT", event}
	eventData, _ := json.Marshal(eventMsg)

	log.Printf("sending event: %s", eventData)
	err = c.WriteMessage(websocket.TextMessage, eventData)
	if err != nil {
		log.Println("write:", err)
		return
	}

	// Wait for interrupt signal
	select {
	case <-done:
		return
	case <-interrupt:
		log.Println("interrupt")

		// Cleanly close the connection
		err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			log.Println("write close:", err)
			return
		}
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		return
	}
}
