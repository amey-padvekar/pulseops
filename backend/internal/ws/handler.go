package ws

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"
)

func allowedOrigin() string {
	origin := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGIN"))
	if origin == "" {
		return "http://localhost:5173"
	}
	return origin
}

// ServeWs upgrades an HTTP request to WebSocket and registers the client
// with the hub.
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	allowed := allowedOrigin()
	upgrader := websocket.Upgrader{
		CheckOrigin: func(req *http.Request) bool {
			return strings.EqualFold(strings.TrimSpace(req.Header.Get("Origin")), allowed)
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade failed: %v", err)
		return
	}

	NewClient(hub, conn)
}
