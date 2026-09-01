package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Hub struct {
	mu       sync.RWMutex
	writeMu  sync.Mutex
	clients  map[*websocket.Conn]bool
	upgrader websocket.Upgrader
}

func NewHub() *Hub {
	return &Hub{clients: map[*websocket.Conn]bool{}, upgrader: websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024, CheckOrigin: func(r *http.Request) bool {
		o := r.Header.Get("Origin")
		if o == "" {
			return true
		}
		u, e := url.Parse(o)
		return e == nil && strings.EqualFold(u.Host, r.Host)
	}}}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.serve(w, r, false)
}
func (h *Hub) ServeAdmin(w http.ResponseWriter, r *http.Request) { h.serve(w, r, true) }
func (h *Hub) serve(w http.ResponseWriter, r *http.Request, admin bool) {
	c, e := h.upgrader.Upgrade(w, r, nil)
	if e != nil {
		return
	}
	h.mu.Lock()
	h.clients[c] = admin
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(h.clients, c); h.mu.Unlock(); c.Close() }()
	_ = c.SetReadDeadline(time.Now().Add(48 * time.Hour))
	c.SetPongHandler(func(string) error { _ = c.SetReadDeadline(time.Now().Add(48 * time.Hour)); return nil })
	for {
		if _, _, e := c.ReadMessage(); e != nil {
			return
		}
	}
}

func (h *Hub) Broadcast(eventType string, data any) {
	payload, _ := json.Marshal(map[string]any{"type": eventType, "data": data})
	h.mu.RLock()
	defer h.mu.RUnlock()
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	for c, admin := range h.clients {
		if sensitiveEvent(eventType) && !admin {
			continue
		}
		_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_ = c.WriteMessage(websocket.TextMessage, payload)
	}
}
func (h *Hub) Count() int { h.mu.RLock(); defer h.mu.RUnlock(); return len(h.clients) }
func sensitiveEvent(t string) bool {
	return strings.HasPrefix(t, "torrent.") || strings.HasPrefix(t, "import.") || strings.HasPrefix(t, "backup.") || strings.HasPrefix(t, "system.") || strings.HasPrefix(t, "filename.") || strings.HasPrefix(t, "enrichment.") || strings.HasPrefix(t, "audio.")
}
