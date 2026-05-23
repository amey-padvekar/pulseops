package ws

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// writeDeadline is applied to every WebSocket write to prevent a slow
	// client from blocking the write pump indefinitely.
	writeDeadline = 10 * time.Second

	// sendBufferSize is the number of outbound messages buffered per client.
	// When the buffer is full the client is considered slow and is dropped.
	sendBufferSize = 256
)

// Hub maintains the set of active WebSocket clients and routes broadcast
// messages to all of them.
//
// The clients map is owned exclusively by the Run goroutine. All other
// goroutines communicate with Run through the register, unregister, and
// broadcast channels — never by accessing the map directly.
type Hub struct {
	// clients holds every currently connected client.
	clients map[*Client]struct{}

	// broadcast receives messages that should be sent to every client.
	broadcast chan []byte

	// register receives new clients that have just connected.
	register chan *Client

	// unregister receives clients that have disconnected or should be dropped.
	unregister chan *Client
}

// Client represents a single WebSocket connection managed by the hub.
type Client struct {
	hub  *Hub
	conn *websocket.Conn

	// send is a buffered channel of outbound messages for this client.
	// The writePump goroutine drains it.
	send chan []byte
}

// NewHub returns an initialised Hub ready to be started with Run.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		broadcast:  make(chan []byte, sendBufferSize),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub event loop. It must be called exactly once, as a
// goroutine, before any clients connect.
//
//	go hub.Run()
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = struct{}{}
			log.Printf("ws hub: client registered total=%d", len(h.clients))

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				_ = client.conn.Close()
				log.Printf("ws hub: client unregistered total=%d", len(h.clients))
			}

		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client send buffer is full — drop and clean up.
					delete(h.clients, client)
					close(client.send)
					_ = client.conn.Close()
					log.Printf("ws hub: slow client dropped total=%d", len(h.clients))
				}
			}
		}
	}
}

// Broadcast sends msg to every connected client in a non-blocking fashion.
// If the hub's broadcast channel is full the message is silently dropped to
// avoid stalling the caller (the telemetry handler).
func (h *Hub) Broadcast(msg []byte) {
	select {
	case h.broadcast <- msg:
	default:
		log.Printf("ws hub: broadcast channel full, message dropped")
	}
}

// NewClient creates a Client for the given connection, registers it with the
// hub, and starts its read and write pumps. This is the single entry point
// used by both the HTTP upgrade handler and integration tests.
func NewClient(h *Hub, conn *websocket.Conn) {
	c := &Client{hub: h, conn: conn, send: make(chan []byte, sendBufferSize)}
	h.register <- c
	go c.writePump()
	go c.readPump()
}

// ServeTestClient is an alias for NewClient used in tests that construct a
// websocket.Conn directly without going through the HTTP upgrade handler.
var ServeTestClient = NewClient

// writePump pumps messages from the client's send channel to the WebSocket
// connection. It runs in its own goroutine per client.
func (c *Client) writePump() {
	defer func() {
		c.hub.unregister <- c
	}()

	for msg := range c.send {
		_ = c.conn.SetWriteDeadline(time.Now().Add(writeDeadline))
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			log.Printf("ws writePump: write error: %v", err)
			return
		}
	}

	// send channel was closed by the hub — send a close frame.
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeDeadline))
	_ = c.conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}

// readPump drains inbound frames from the WebSocket connection. The MVP
// frontend does not send data, so this only handles pings and close frames.
// It runs in its own goroutine per client and triggers unregister on any
// read error or when the connection is closed by the client.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
	}()

	// No read deadline — the connection stays open until the client closes it.
	// Set a generous max message size to guard against malformed input.
	c.conn.SetReadLimit(512)

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
				websocket.CloseNoStatusReceived,
			) {
				log.Printf("ws readPump: unexpected close: %v", err)
			}
			return
		}
		// Messages from the client are intentionally ignored for the MVP.
	}
}
