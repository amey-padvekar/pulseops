package demo

import (
	"strings"
	"testing"
	"time"
)

func TestCatalogNamesOnlyRealWindowsServices(t *testing.T) {
	got := Catalog()
	if len(got) != 4 {
		t.Fatalf("catalog size = %d, want 4", len(got))
	}

	wantService := map[string]string{
		"defender": "WinDefend",
		"mysql":    "MySQL80",
		"iis":      "W3SVC",
		"spooler":  "Spooler",
	}
	for _, s := range got {
		want, ok := wantService[s.Key]
		if !ok {
			t.Errorf("unexpected scenario key %q", s.Key)
			continue
		}
		if s.ServiceName != want {
			t.Errorf("scenario %q service = %q, want %q", s.Key, s.ServiceName, want)
		}
		if strings.TrimSpace(s.Label) == "" {
			t.Errorf("scenario %q has empty label", s.Key)
		}
		if s.Mode != ModeSimulated && s.Mode != ModeReal {
			t.Errorf("scenario %q has invalid mode %q", s.Key, s.Mode)
		}
		if len(s.Logs) == 0 {
			t.Errorf("scenario %q has no flavored logs", s.Key)
		}
	}
}

func TestDefaultScenarioIsDefenderSimulated(t *testing.T) {
	d := Default()
	if d.Key != DefaultScenarioKey {
		t.Fatalf("default scenario key = %q, want %q", d.Key, DefaultScenarioKey)
	}
	if DefaultScenarioKey != "defender" {
		t.Fatalf("DefaultScenarioKey = %q, want defender", DefaultScenarioKey)
	}
	if d.ServiceName != "WinDefend" {
		t.Errorf("default service = %q, want WinDefend", d.ServiceName)
	}
	// Defender is simulated-only because tamper protection may block a real restart.
	if d.Mode != ModeSimulated {
		t.Errorf("default mode = %q, want %q", d.Mode, ModeSimulated)
	}
}

func TestByKeyUnknownReturnsFalse(t *testing.T) {
	if s, ok := ByKey("does-not-exist"); ok {
		t.Fatalf("ByKey(does-not-exist) = (%+v, true), want (_, false)", s)
	}
}

func TestRecentLogsMatchEventLogShape(t *testing.T) {
	at := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)

	s, ok := ByKey("spooler")
	if !ok {
		t.Fatal("spooler scenario missing from catalog")
	}

	lines := s.RecentLogs(at)
	if len(lines) != len(s.Logs) {
		t.Fatalf("rendered %d log lines, want %d", len(lines), len(s.Logs))
	}

	for _, line := range lines {
		// Real collector shape: timestamp|provider|eventId|level|message
		parts := strings.SplitN(line, "|", 5)
		if len(parts) != 5 {
			t.Fatalf("log line %q has %d fields, want 5 (timestamp|provider|eventId|level|message)", line, len(parts))
		}
		if parts[0] != "2026-06-09T12:00:00Z" {
			t.Errorf("timestamp field = %q, want 2026-06-09T12:00:00Z", parts[0])
		}
		if strings.TrimSpace(parts[4]) == "" {
			t.Errorf("log line %q has empty message", line)
		}
	}
}
