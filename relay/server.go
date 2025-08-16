package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nbd-wtf/go-nostr"
)

type Server struct {
	clients    map[*Client]bool
	clientsMux sync.RWMutex
	events     map[string]*nostr.Event
	eventsMux  sync.RWMutex
	upgrader   websocket.Upgrader
	accessLog  *log.Logger
	eventLog   *log.Logger
}

func NewServer() *Server {
	return &Server{clients: map[*Client]bool{}, events: map[string]*nostr.Event{}, upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}, accessLog: log.New(os.Stdout, "ACCESS ", log.LstdFlags)}
}

// InitAccessLog sets file logging (append) plus stdout
// InitLogDir creates log directory and sets loggers (access, server, events)
func (s *Server) InitLogDir(dir string) error {
	if dir == "" {
		dir = "log"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	open := func(name string) (*os.File, error) {
		return os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	}
	acc, err := open("access.log")
	if err != nil {
		return fmt.Errorf("open access.log: %w", err)
	}
	s.accessLog = log.New(io.MultiWriter(os.Stdout, acc), "ACCESS ", log.LstdFlags)

	srv, err := open("server.log")
	if err != nil {
		return fmt.Errorf("open server.log: %w", err)
	}
	log.SetOutput(io.MultiWriter(os.Stdout, srv))
	log.SetFlags(log.LstdFlags)

	evt, err := open("events.log")
	if err != nil {
		return fmt.Errorf("open events.log: %w", err)
	}
	s.eventLog = log.New(io.MultiWriter(os.Stdout, evt), "EVENT  ", log.LstdFlags)

	return nil
}

// WithAccessLog middleware
func (s *Server) WithAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(lrw, r)
		if s.accessLog != nil {
			ua := r.Header.Get("User-Agent")
			s.accessLog.Printf("%s %s %d %d %s remote=%s ua=\"%s\"", r.Method, r.URL.Path, lrw.status, lrw.bytes, time.Since(start), r.RemoteAddr, ua)
		}
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status, bytes int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.status = code
	lrw.ResponseWriter.WriteHeader(code)
}
func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	if lrw.status == 0 {
		lrw.status = http.StatusOK
	}
	n, err := lrw.ResponseWriter.Write(b)
	lrw.bytes += n
	return n, err
}

type Client struct {
	conn          *websocket.Conn
	server        *Server
	send          chan []byte
	subscriptions map[string]*Subscription
	subsMux       sync.RWMutex
}
type Subscription struct {
	ID      string
	Filters []nostr.Filter
}

func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	c := &Client{conn: conn, server: s, send: make(chan []byte, 256), subscriptions: map[string]*Subscription{}}
	s.clientsMux.Lock()
	s.clients[c] = true
	s.clientsMux.Unlock()
	log.Printf("Client connected: %s", conn.RemoteAddr())
	go c.writePump()
	go c.readPump()
}

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
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); return nil })
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
		c.handleMessage(msg)
	}
}
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() { ticker.Stop(); c.conn.Close() }()
	for {
		select {
		case m, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, m); err != nil {
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
func (c *Client) handleMessage(b []byte) {
	var msg []interface{}
	if err := json.Unmarshal(b, &msg); err != nil {
		log.Printf("JSON unmarshal error: %v", err)
		c.sendNotice("Invalid message format")
		return
	}
	if len(msg) == 0 {
		c.sendNotice("Empty message")
		return
	}
	t, ok := msg[0].(string)
	if !ok {
		c.sendNotice("Invalid message type")
		return
	}
	switch t {
	case "EVENT":
		c.handleEvent(msg)
	case "REQ":
		c.handleReq(msg)
	case "CLOSE":
		c.handleClose(msg)
	default:
		c.sendNotice("Unknown message type: " + t)
	}
}
func (c *Client) handleEvent(msg []interface{}) {
	if len(msg) < 2 {
		c.sendNotice("Invalid EVENT message")
		return
	}
	raw, err := json.Marshal(msg[1])
	if err != nil {
		c.sendNotice("Invalid event data")
		return
	}
	var ev nostr.Event
	if err := json.Unmarshal(raw, &ev); err != nil {
		c.sendNotice("Invalid event format")
		return
	}
	ok, err := ev.CheckSignature()
	if err != nil || !ok {
		c.sendOK(ev.ID, false, "invalid signature")
		return
	}
	c.server.eventsMux.Lock()
	c.server.events[ev.ID] = &ev
	c.server.eventsMux.Unlock()
	c.sendOK(ev.ID, true, "")
	c.server.broadcastEvent(&ev)
	if c.server.eventLog != nil {
		c.server.eventLog.Printf("stored id=%s kind=%d pub=%s size=%d", ev.ID, ev.Kind, ev.PubKey, len(ev.Content))
	}
	log.Printf("Event stored: %s", ev.ID)
}
func (c *Client) handleReq(msg []interface{}) {
	if len(msg) < 2 {
		c.sendNotice("Invalid REQ message")
		return
	}
	id, ok := msg[1].(string)
	if !ok {
		c.sendNotice("Invalid subscription ID")
		return
	}
	var filters []nostr.Filter
	for i := 2; i < len(msg); i++ {
		data, err := json.Marshal(msg[i])
		if err != nil {
			continue
		}
		var f nostr.Filter
		if err := json.Unmarshal(data, &f); err == nil {
			filters = append(filters, f)
		}
	}
	c.subsMux.Lock()
	c.subscriptions[id] = &Subscription{ID: id, Filters: filters}
	c.subsMux.Unlock()
	c.server.eventsMux.RLock()
	for _, ev := range c.server.events {
		if c.eventMatchesFilters(ev, filters) {
			c.sendEvent(id, ev)
		}
	}
	c.server.eventsMux.RUnlock()
	c.sendEOSE(id)
	log.Printf("Subscription created: %s", id)
}
func (c *Client) handleClose(msg []interface{}) {
	if len(msg) < 2 {
		c.sendNotice("Invalid CLOSE message")
		return
	}
	id, ok := msg[1].(string)
	if !ok {
		c.sendNotice("Invalid subscription ID")
		return
	}
	c.subsMux.Lock()
	delete(c.subscriptions, id)
	c.subsMux.Unlock()
	log.Printf("Subscription closed: %s", id)
}
func (c *Client) eventMatchesFilters(ev *nostr.Event, fs []nostr.Filter) bool {
	for _, f := range fs {
		if c.eventMatchesFilter(ev, f) {
			return true
		}
	}
	return false
}
func (c *Client) eventMatchesFilter(ev *nostr.Event, f nostr.Filter) bool {
	if len(f.IDs) > 0 {
		ok := false
		for _, id := range f.IDs {
			if len(id) > 0 && len(ev.ID) >= len(id) && ev.ID[:len(id)] == id {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(f.Authors) > 0 {
		ok := false
		for _, a := range f.Authors {
			if len(a) > 0 && len(ev.PubKey) >= len(a) && ev.PubKey[:len(a)] == a {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(f.Kinds) > 0 {
		ok := false
		for _, k := range f.Kinds {
			if ev.Kind == k {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if f.Since != nil && ev.CreatedAt.Time().Before(f.Since.Time()) {
		return false
	}
	if f.Until != nil && ev.CreatedAt.Time().After(f.Until.Time()) {
		return false
	}
	for tagName, vals := range f.Tags {
		if len(vals) == 0 {
			continue
		}
		found := false
		for _, tag := range ev.Tags {
			if len(tag) >= 2 && tag[0] == tagName {
				for _, v := range vals {
					if tag[1] == v {
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
func (s *Server) broadcastEvent(ev *nostr.Event) {
	s.clientsMux.RLock()
	defer s.clientsMux.RUnlock()
	for c := range s.clients {
		c.subsMux.RLock()
		for id, sub := range c.subscriptions {
			if c.eventMatchesFilters(ev, sub.Filters) {
				c.sendEvent(id, ev)
			}
		}
		c.subsMux.RUnlock()
	}
}
func (c *Client) sendEvent(id string, ev *nostr.Event) { c.sendMsg([]interface{}{"EVENT", id, ev}) }
func (c *Client) sendOK(id string, accepted bool, m string) {
	c.sendMsg([]interface{}{"OK", id, accepted, m})
}
func (c *Client) sendEOSE(id string)  { c.sendMsg([]interface{}{"EOSE", id}) }
func (c *Client) sendNotice(m string) { c.sendMsg([]interface{}{"NOTICE", m}) }
func (c *Client) sendMsg(v []interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("marshal error: %v", err)
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
