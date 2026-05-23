package ws_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/ws"
	"github.com/gorilla/websocket"
)

// dialHub starts a Hub, registers one WebSocket client via the test upgrader,
// and returns the client-side connection alongside a cleanup function.
func dialHub(t *testing.T, h *ws.Hub) (*websocket.Conn, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade error: %v", err)
			return
		}
		ws.ServeTestClient(h, conn)
	}))

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}

	return conn, func() {
		conn.Close()
		srv.Close()
	}
}

func TestHub_BroadcastReachesClient(t *testing.T) {
	h := ws.NewHub()
	go h.Run()

	conn, cleanup := dialHub(t, h)
	defer cleanup()

	// Give the hub a moment to register the client.
	time.Sleep(50 * time.Millisecond)

	want := `{"deviceId":"LAPTOP-22","serviceStatus":"running"}`
	h.Broadcast([]byte(want))

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(msg) != want {
		t.Errorf("got %q, want %q", string(msg), want)
	}
}

func TestHub_BroadcastWithNoClients(t *testing.T) {
	h := ws.NewHub()
	go h.Run()
	// Should not block or panic when there are no connected clients.
	h.Broadcast([]byte(`{"deviceId":"LAPTOP-22"}`))
}

func TestHub_BroadcastChannelFullDropsSilently(t *testing.T) {
	h := ws.NewHub()
	go h.Run()
	// Flood the broadcast channel beyond its buffer — must not deadlock.
	for i := 0; i < 300; i++ {
		h.Broadcast([]byte(`{"deviceId":"X"}`))
	}
}
