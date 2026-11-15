package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"slices"
	"sync"
	"time"

	gws "github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = pongWait * 9 / 10
	maxMessageSize = 4096
	sendBufferSize = 32
)

// Hub tracks WebSocket connections per user and pushes events to them.
// A user may have several connections (multiple tabs/devices).
type Hub struct {
	mu       sync.RWMutex
	clients  map[int64]map[*Client]struct{}
	upgrader gws.Upgrader
}

type Client struct {
	hub       *Hub
	conn      *gws.Conn
	userID    int64
	send      chan []byte
	closeOnce sync.Once
}

func NewHub(allowedOrigins []string) *Hub {
	allowAll := slices.Contains(allowedOrigins, "*")
	return &Hub{
		clients: make(map[int64]map[*Client]struct{}),
		upgrader: gws.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				return origin == "" || allowAll || slices.Contains(allowedOrigins, origin)
			},
		},
	}
}

// ServeWS upgrades the request and registers the connection for userID.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, userID int64) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade: %v", err) // upgrader already wrote the HTTP error
		return
	}

	c := &Client{hub: h, conn: conn, userID: userID, send: make(chan []byte, sendBufferSize)}
	h.add(c)

	go c.writePump()
	go c.readPump()
}

// SendToUser delivers an event (JSON-encoded) to every connection of userID.
func (h *Hub) SendToUser(userID int64, event any) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("websocket marshal: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients[userID] {
		select {
		case c.send <- payload:
		default:
			go h.remove(c) // buffer full: drop the slow client
		}
	}
}

// Close disconnects every client (used on shutdown).
func (h *Hub) Close() {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, set := range h.clients {
		for c := range set {
			_ = c.conn.Close()
		}
	}
}

func (h *Hub) add(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set, ok := h.clients[c.userID]
	if !ok {
		set = make(map[*Client]struct{})
		h.clients[c.userID] = set
	}
	set[c] = struct{}{}
}

func (h *Hub) remove(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.clients[c.userID]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.clients, c.userID)
		}
	}
	c.closeOnce.Do(func() { close(c.send) })
}

func (c *Client) readPump() {
	defer func() {
		c.hub.remove(c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		// Incoming client frames are ignored for now; messages are sent via the REST API.
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(gws.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(gws.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(gws.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
