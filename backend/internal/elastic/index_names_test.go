package elastic

import (
	"testing"
	"time"
)

func TestIndexName(t *testing.T) {

	ts := time.Date(
		2026,
		5,
		25,
		10,
		30,
		0,
		0,
		time.UTC,
	)

	got := IndexName("telemetry-events", ts)

	want := "telemetry-events-2026.05.25"

	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
