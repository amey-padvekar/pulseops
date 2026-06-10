package agentbuilder

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/elastic"
)

type capturedSearch struct {
	path string
	body map[string]any
}

func TestRetrieveAndSummarizeEvidence_UsesScopedQueriesAndSeparateIndices(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []capturedSearch
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		payload, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		var body map[string]any
		_ = json.Unmarshal(payload, &body)

		mu.Lock()
		calls = append(calls, capturedSearch{path: r.URL.Path, body: body})
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":{"hits":[{"_source":{"timestamp":"2026-06-01T12:00:00Z","serviceStatus":"stopped","heartbeat":true,"message":"service stopped","source":"endpoint-agent","severity":"high","reason":"service stopped"}}]}}`))
	}))
	defer server.Close()

	esClient, err := elastic.NewClient(&elastic.Config{
		Endpoint: server.URL,
		APIKey:   "test-key",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("elastic.NewClient error: %v", err)
	}

	start := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) // 2h window, should be bounded to 30m.

	summary, err := RetrieveAndSummarizeEvidence(context.Background(), esClient, ElasticContextHints{
		DeviceID:       "DEV-01",
		IncidentID:     "INC-01",
		ServiceName:    "OpenVPNService",
		TimeRangeStart: start,
		TimeRangeEnd:   end,
		IndexPatterns:  []string{"telemetry-events-*", "incident-events-*", "endpoint-logs-*"},
	})
	if err != nil {
		t.Fatalf("RetrieveAndSummarizeEvidence error: %v", err)
	}

	if !strings.Contains(summary, "Evidence summary") {
		t.Fatalf("summary missing header: %s", summary)
	}
	if !strings.Contains(summary, "QueryWindow") {
		t.Fatalf("summary missing query window: %s", summary)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(calls) != 3 {
		t.Fatalf("search call count = %d, want 3", len(calls))
	}

	if !strings.Contains(calls[0].path, "/telemetry-events-*") {
		t.Fatalf("telemetry query path = %q, expected telemetry index", calls[0].path)
	}
	if !strings.Contains(calls[1].path, "/incident-events-*") {
		t.Fatalf("incidents query path = %q, expected incidents index", calls[1].path)
	}
	if !strings.Contains(calls[2].path, "/endpoint-logs-*") {
		t.Fatalf("logs query path = %q, expected logs index", calls[2].path)
	}

	for i, c := range calls {
		must := mustClausesFromBody(c.body)
		if !containsTerm(must, elastic.FieldServiceName, "OpenVPNService") {
			t.Fatalf("call[%d] missing serviceName filter in must clauses: %+v", i, must)
		}
		if i != 1 && !containsTerm(must, elastic.FieldDeviceID, "DEV-01") {
			t.Fatalf("call[%d] missing deviceId filter in must clauses: %+v", i, must)
		}
		if i == 1 && !containsTerm(must, elastic.FieldIncidentID, "INC-01") {
			t.Fatalf("call[%d] missing incidentId filter in must clauses: %+v", i, must)
		}

		gte, lte, ok := extractTimeRange(must)
		if !ok {
			t.Fatalf("call[%d] missing timestamp range clause: %+v", i, must)
		}

		gteTime, err := time.Parse(time.RFC3339, gte)
		if err != nil {
			t.Fatalf("call[%d] invalid gte timestamp %q: %v", i, gte, err)
		}
		lteTime, err := time.Parse(time.RFC3339, lte)
		if err != nil {
			t.Fatalf("call[%d] invalid lte timestamp %q: %v", i, lte, err)
		}

		if lteTime.Sub(gteTime) > 30*time.Minute {
			t.Fatalf("call[%d] range exceeds 30m bound: %s to %s", i, gte, lte)
		}
	}
}

func mustClausesFromBody(body map[string]any) []any {
	query, ok := body["query"].(map[string]any)
	if !ok {
		return nil
	}
	boolNode, ok := query["bool"].(map[string]any)
	if !ok {
		return nil
	}
	must, _ := boolNode["must"].([]any)
	return must
}

func containsTerm(clauses []any, field string, expected string) bool {
	for _, c := range clauses {
		clause, ok := c.(map[string]any)
		if !ok {
			continue
		}
		termNode, ok := clause["term"].(map[string]any)
		if !ok {
			continue
		}
		if v, ok := termNode[field]; ok && v == expected {
			return true
		}
	}
	return false
}

func extractTimeRange(clauses []any) (string, string, bool) {
	for _, c := range clauses {
		clause, ok := c.(map[string]any)
		if !ok {
			continue
		}
		rangeNode, ok := clause["range"].(map[string]any)
		if !ok {
			continue
		}
		tsNode, ok := rangeNode[elastic.FieldTimestamp].(map[string]any)
		if !ok {
			continue
		}
		gte, _ := tsNode["gte"].(string)
		lte, _ := tsNode["lte"].(string)
		if gte != "" && lte != "" {
			return gte, lte, true
		}
	}
	return "", "", false
}
