package relay

import (
	"encoding/json"
	"fmt"
	"net/http"
)

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

func (s *Server) HandleRelayInfo(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Accept") == "application/nostr+json" {
		info := RelayInfo{Name: "Nostr Sample Relay", Description: "A sample Nostr relay implementation in Go", Contact: "admin@example.com", SupportedNips: []int{1, 2, 9, 11, 12, 15, 16, 20, 22}, Software: "nostr-sample", Version: "1.0.0", Limitation: Limitation{MaxMessageLength: 16384, MaxSubscriptions: 20, MaxFilters: 100, MaxLimit: 5000, MaxSubidLength: 100, MaxEventTags: 100, MaxContentLength: 8196, MinPowDifficulty: 0, AuthRequired: false, PaymentRequired: false, RestrictedWrites: false}}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		_ = json.NewEncoder(w).Encode(info)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>Nostr Sample Relay</title></head><body><h1>Nostr Sample Relay</h1><p>This is a sample Nostr relay implementation in Go.</p><p>WebSocket endpoint: ws://%s/ws</p><p>To get relay information in JSON format, send a request with Accept header set to "application/nostr+json"</p></body></html>`, r.Host)
}
