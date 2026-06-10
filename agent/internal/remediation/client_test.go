package remediation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_Fetch_ReturnsScopedCommands(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pendingCommandsResponse{
			DeviceID: "dev-1",
			Commands: []Command{
				{IncidentID: "INC-1", DeviceID: "dev-1", RequestID: "rem-1", DispatchedAt: time.Now().UTC(),
					Actions: []Action{{ActionID: "restart_service", Target: "OpenVPNService"}}},
				// A misrouted command for another device must be dropped client-side.
				{IncidentID: "INC-2", DeviceID: "dev-OTHER", RequestID: "rem-2"},
			},
		})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "dev-1", srv.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	cmds, err := c.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotPath != "/devices/dev-1/commands" {
		t.Fatalf("unexpected request path: %q", gotPath)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 scoped command, got %d", len(cmds))
	}
	if cmds[0].IncidentID != "INC-1" || cmds[0].DeviceID != "dev-1" {
		t.Fatalf("wrong command returned: %+v", cmds[0])
	}
}

func TestClient_Fetch_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "dev-1", srv.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Fetch(context.Background()); err == nil {
		t.Fatal("expected an error for a 500 response")
	}
}

func TestJoinCommandsEndpoint(t *testing.T) {
	cases := []struct {
		base, device, want string
	}{
		{"http://localhost:8080", "dev-1", "http://localhost:8080/devices/dev-1/commands"},
		{"http://localhost:8080/", "dev-1", "http://localhost:8080/devices/dev-1/commands"},
		{"https://host/api/", "DEV AGENT", "https://host/api/devices/DEV%20AGENT/commands"},
	}
	for _, tc := range cases {
		got, err := joinCommandsEndpoint(tc.base, tc.device)
		if err != nil {
			t.Fatalf("joinCommandsEndpoint(%q,%q): %v", tc.base, tc.device, err)
		}
		if got != tc.want {
			t.Fatalf("join(%q,%q): got %q want %q", tc.base, tc.device, got, tc.want)
		}
	}

	if _, err := joinCommandsEndpoint("http://host", ""); err == nil {
		t.Fatal("expected error for empty device id")
	}
	if _, err := joinCommandsEndpoint("not-a-url", "dev-1"); err == nil {
		t.Fatal("expected error for non-absolute base URL")
	}
}
