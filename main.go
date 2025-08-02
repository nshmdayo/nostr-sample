package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nbd-wtf/go-nostr"
)

// NostrServer represents the main server structure
type NostrServer struct {
	clients    map[*Client]bool
	clientsMux sync.RWMutex
	events     map[string]*nostr.Event
	eventsMux  sync.RWMutex
	upgrader   websocket.Upgrader
}

// Client represents a WebSocket client connection
type Client struct {
	conn          *websocket.Conn
	server        *NostrServer
	send          chan []byte
	subscriptions map[string]*Subscription
	subsMux       sync.RWMutex
}

// Subscription represents a client subscription
type Subscription struct {
	ID      string
	Filters []nostr.Filter
}

// NewNostrServer creates a new Nostr server instance
func NewNostrServer() *NostrServer {
	return &NostrServer{
		clients: make(map[*Client]bool),
		events:  make(map[string]*nostr.Event),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
			},
		},
	}
}

// HandleWebSocket handles WebSocket connections
func (s *NostrServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := &Client{
		conn:          conn,
		server:        s,
		send:          make(chan []byte, 256),
		subscriptions: make(map[string]*Subscription),
	}

	s.clientsMux.Lock()
	s.clients[client] = true
	s.clientsMux.Unlock()

	log.Printf("Client connected: %s", conn.RemoteAddr())

	// Start goroutines for reading and writing
	go client.writePump()
	go client.readPump()
}

// readPump handles reading messages from the WebSocket connection
func (c *Client) readPump() {
	defer func() {
		c.server.clientsMux.Lock()
		delete(c.server.clients, c)
		c.server.clientsMux.Unlock()
		c.conn.Close()
		log.Printf("Client disconnected: %s", c.conn.RemoteAddr())
	}()

	c.conn.SetReadLimit(512 * 1024)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		c.handleMessage(message)
	}
}

// writePump handles writing messages to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("WebSocket write error: %v", err)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes incoming messages from clients
func (c *Client) handleMessage(message []byte) {
	var msg []interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Printf("JSON unmarshal error: %v", err)
		c.sendNotice("Invalid message format")
		return
	}

	if len(msg) == 0 {
		c.sendNotice("Empty message")
		return
	}

	msgType, ok := msg[0].(string)
	if !ok {
		c.sendNotice("Invalid message type")
		return
	}

	switch msgType {
	case "EVENT":
		c.handleEvent(msg)
	case "REQ":
		c.handleReq(msg)
	case "CLOSE":
		c.handleClose(msg)
	default:
		c.sendNotice(fmt.Sprintf("Unknown message type: %s", msgType))
	}
}

// handleEvent processes EVENT messages
func (c *Client) handleEvent(msg []interface{}) {
	if len(msg) < 2 {
		c.sendNotice("Invalid EVENT message")
		return
	}

	eventData, err := json.Marshal(msg[1])
	if err != nil {
		c.sendNotice("Invalid event data")
		return
	}

	var event nostr.Event
	if err := json.Unmarshal(eventData, &event); err != nil {
		c.sendNotice("Invalid event format")
		return
	}

	// Validate event signature
	ok, err := event.CheckSignature()
	if err != nil || !ok {
		c.sendOK(event.ID, false, "invalid signature")
		return
	}

	// Store the event
	c.server.eventsMux.Lock()
	c.server.events[event.ID] = &event
	c.server.eventsMux.Unlock()

	// Send OK response
	c.sendOK(event.ID, true, "")

	// Broadcast to matching subscriptions
	c.server.broadcastEvent(&event)

	log.Printf("Event stored: %s", event.ID)
}

// handleReq processes REQ messages (subscriptions)
func (c *Client) handleReq(msg []interface{}) {
	if len(msg) < 2 {
		c.sendNotice("Invalid REQ message")
		return
	}

	subID, ok := msg[1].(string)
	if !ok {
		c.sendNotice("Invalid subscription ID")
		return
	}

	var filters []nostr.Filter
	for i := 2; i < len(msg); i++ {
		filterData, err := json.Marshal(msg[i])
		if err != nil {
			continue
		}

		var filter nostr.Filter
		if err := json.Unmarshal(filterData, &filter); err != nil {
			continue
		}

		filters = append(filters, filter)
	}

	// Store subscription
	c.subsMux.Lock()
	c.subscriptions[subID] = &Subscription{
		ID:      subID,
		Filters: filters,
	}
	c.subsMux.Unlock()

	// Send matching events
	c.server.eventsMux.RLock()
	for _, event := range c.server.events {
		if c.eventMatchesFilters(event, filters) {
			c.sendEvent(subID, event)
		}
	}
	c.server.eventsMux.RUnlock()

	// Send EOSE (End of Stored Events)
	c.sendEOSE(subID)

	log.Printf("Subscription created: %s", subID)
}

// handleClose processes CLOSE messages
func (c *Client) handleClose(msg []interface{}) {
	if len(msg) < 2 {
		c.sendNotice("Invalid CLOSE message")
		return
	}

	subID, ok := msg[1].(string)
	if !ok {
		c.sendNotice("Invalid subscription ID")
		return
	}

	c.subsMux.Lock()
	delete(c.subscriptions, subID)
	c.subsMux.Unlock()

	log.Printf("Subscription closed: %s", subID)
}

// broadcastEvent sends an event to all clients with matching subscriptions
func (s *NostrServer) broadcastEvent(event *nostr.Event) {
	s.clientsMux.RLock()
	defer s.clientsMux.RUnlock()

	for client := range s.clients {
		client.subsMux.RLock()
		for subID, sub := range client.subscriptions {
			if client.eventMatchesFilters(event, sub.Filters) {
				client.sendEvent(subID, event)
			}
		}
		client.subsMux.RUnlock()
	}
}

// eventMatchesFilters checks if an event matches any of the given filters
func (c *Client) eventMatchesFilters(event *nostr.Event, filters []nostr.Filter) bool {
	for _, filter := range filters {
		if c.eventMatchesFilter(event, filter) {
			return true
		}
	}
	return false
}

// eventMatchesFilter checks if an event matches a single filter
func (c *Client) eventMatchesFilter(event *nostr.Event, filter nostr.Filter) bool {
	// Check IDs
	if len(filter.IDs) > 0 {
		found := false
		for _, id := range filter.IDs {
			if len(id) > 0 && len(event.ID) >= len(id) && event.ID[:len(id)] == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check Authors
	if len(filter.Authors) > 0 {
		found := false
		for _, author := range filter.Authors {
			if len(author) > 0 && len(event.PubKey) >= len(author) && event.PubKey[:len(author)] == author {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check Kinds
	if len(filter.Kinds) > 0 {
		found := false
		for _, kind := range filter.Kinds {
			if event.Kind == kind {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check Since
	if filter.Since != nil && event.CreatedAt.Time().Before(filter.Since.Time()) {
		return false
	}

	// Check Until
	if filter.Until != nil && event.CreatedAt.Time().After(filter.Until.Time()) {
		return false
	}

	// Check Tags
	for tagName, tagValues := range filter.Tags {
		if len(tagValues) == 0 {
			continue
		}

		found := false
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == tagName {
				for _, targetValue := range tagValues {
					if tag[1] == targetValue {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// sendEvent sends an EVENT message to the client
func (c *Client) sendEvent(subID string, event *nostr.Event) {
	msg := []interface{}{"EVENT", subID, event}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling event: %v", err)
		return
	}

	select {
	case c.send <- data:
	default:
		close(c.send)
		c.server.clientsMux.Lock()
		delete(c.server.clients, c)
		c.server.clientsMux.Unlock()
	}
}

// sendOK sends an OK message to the client
func (c *Client) sendOK(eventID string, accepted bool, message string) {
	msg := []interface{}{"OK", eventID, accepted, message}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling OK: %v", err)
		return
	}

	select {
	case c.send <- data:
	default:
		close(c.send)
		c.server.clientsMux.Lock()
		delete(c.server.clients, c)
		c.server.clientsMux.Unlock()
	}
}

// sendEOSE sends an EOSE message to the client
func (c *Client) sendEOSE(subID string) {
	msg := []interface{}{"EOSE", subID}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling EOSE: %v", err)
		return
	}

	select {
	case c.send <- data:
	default:
		close(c.send)
		c.server.clientsMux.Lock()
		delete(c.server.clients, c)
		c.server.clientsMux.Unlock()
	}
}

// sendNotice sends a NOTICE message to the client
func (c *Client) sendNotice(message string) {
	msg := []interface{}{"NOTICE", message}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling notice: %v", err)
		return
	}

	select {
	case c.send <- data:
	default:
		close(c.send)
		c.server.clientsMux.Lock()
		delete(c.server.clients, c)
		c.server.clientsMux.Unlock()
	}
}

// RelayInfo represents relay information
type RelayInfo struct {
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Pubkey        string     `json:"pubkey"`
	Contact       string     `json:"contact"`
	SupportedNips []int      `json:"supported_nips"`
	Software      string     `json:"software"`
	Version       string     `json:"version"`
	Limitation    Limitation `json:"limitation"`
}

// Limitation represents relay limitations
type Limitation struct {
	MaxMessageLength int  `json:"max_message_length"`
	MaxSubscriptions int  `json:"max_subscriptions"`
	MaxFilters       int  `json:"max_filters"`
	MaxLimit         int  `json:"max_limit"`
	MaxSubidLength   int  `json:"max_subid_length"`
	MaxEventTags     int  `json:"max_event_tags"`
	MaxContentLength int  `json:"max_content_length"`
	MinPowDifficulty int  `json:"min_pow_difficulty"`
	AuthRequired     bool `json:"auth_required"`
	PaymentRequired  bool `json:"payment_required"`
	RestrictedWrites bool `json:"restricted_writes"`
}

// HandleRelayInfo handles requests for relay information
func (s *NostrServer) HandleRelayInfo(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Accept") == "application/nostr+json" {
		info := RelayInfo{
			Name:          "Nostr Sample Relay",
			Description:   "A sample Nostr relay implementation in Go",
			Contact:       "admin@example.com",
			SupportedNips: []int{1, 2, 9, 11, 12, 15, 16, 20, 22},
			Software:      "nostr-sample",
			Version:       "1.0.0",
			Limitation: Limitation{
				MaxMessageLength: 16384,
				MaxSubscriptions: 20,
				MaxFilters:       100,
				MaxLimit:         5000,
				MaxSubidLength:   100,
				MaxEventTags:     100,
				MaxContentLength: 8196,
				MinPowDifficulty: 0,
				AuthRequired:     false,
				PaymentRequired:  false,
				RestrictedWrites: false,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET")

		json.NewEncoder(w).Encode(info)
		return
	}

	// Serve basic HTML page for browsers
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
	<title>Nostr Sample Relay</title>
</head>
<body>
	<h1>Nostr Sample Relay</h1>
	<p>This is a sample Nostr relay implementation in Go.</p>
	<p>WebSocket endpoint: ws://%s/</p>
	<p>To get relay information in JSON format, send a request with Accept header set to "application/nostr+json"</p>
</body>
</html>
`, r.Host)
}

func main() {
	server := NewNostrServer()

	http.HandleFunc("/", server.HandleRelayInfo)
	http.HandleFunc("/ws", server.HandleWebSocket)

	port := ":8080"
	log.Printf("Nostr relay server starting on port %s", port)
	log.Printf("WebSocket endpoint: ws://localhost%s/ws", port)
	log.Printf("Relay info: http://localhost%s/", port)

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal("Server failed to start:", err)
	}
}
