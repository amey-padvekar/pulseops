package remediation

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/certainelf/pulseops/agent/internal/config"
)

type fakeFetcher struct {
	mu      sync.Mutex
	batches [][]Command
	errs    []error
	calls   int
}

func (f *fakeFetcher) Fetch(context.Context) ([]Command, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.calls
	f.calls++
	var cmds []Command
	if i < len(f.batches) {
		cmds = f.batches[i]
	}
	var err error
	if i < len(f.errs) {
		err = f.errs[i]
	}
	return cmds, err
}

type recordingHandler struct {
	mu       sync.Mutex
	handled  []Command
	returns  error
	signalCh chan struct{}
}

func (h *recordingHandler) Handle(_ context.Context, cmd Command) error {
	h.mu.Lock()
	h.handled = append(h.handled, cmd)
	h.mu.Unlock()
	if h.signalCh != nil {
		h.signalCh <- struct{}{}
	}
	return h.returns
}

func quietLogger() *log.Logger { return log.New(io.Discard, "", 0) }

func testConfig() config.RuntimeConfig {
	return config.RuntimeConfig{DeviceID: "dev-1", RemediationPollInterval: 5 * time.Millisecond}
}

func TestPoller_PollOnce_LogsAndHandlesEachCommand(t *testing.T) {
	fetcher := &fakeFetcher{batches: [][]Command{{
		{IncidentID: "INC-1", DeviceID: "dev-1", RequestID: "rem-1"},
		{IncidentID: "INC-2", DeviceID: "dev-1", RequestID: "rem-2"},
	}}}
	handler := &recordingHandler{}
	p := NewPoller(testConfig(), fetcher, handler, quietLogger())

	if err := p.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	if len(handler.handled) != 2 {
		t.Fatalf("expected 2 handled commands, got %d", len(handler.handled))
	}
}

func TestPoller_PollOnce_NilHandlerIsReceiptOnly(t *testing.T) {
	fetcher := &fakeFetcher{batches: [][]Command{{{IncidentID: "INC-1", DeviceID: "dev-1"}}}}
	p := NewPoller(testConfig(), fetcher, nil, quietLogger())

	if err := p.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce with nil handler should not error: %v", err)
	}
}

func TestPoller_PollOnce_FetchErrorPropagates(t *testing.T) {
	fetcher := &fakeFetcher{errs: []error{errors.New("network down")}}
	p := NewPoller(testConfig(), fetcher, &recordingHandler{}, quietLogger())

	if err := p.pollOnce(context.Background()); err == nil {
		t.Fatal("expected fetch error to propagate")
	}
}

func TestPoller_Run_DeliversThenStopsOnCancel(t *testing.T) {
	fetcher := &fakeFetcher{batches: [][]Command{{{IncidentID: "INC-1", DeviceID: "dev-1", RequestID: "rem-1"}}}}
	handler := &recordingHandler{signalCh: make(chan struct{}, 1)}
	p := NewPoller(testConfig(), fetcher, handler, quietLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	select {
	case <-handler.signalCh:
		// command delivered via the running loop
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("timed out waiting for the poller to deliver a command")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error on cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}

func TestNextBackoff_DoublesAndCaps(t *testing.T) {
	if got := nextBackoff(5 * time.Second); got != 10*time.Second {
		t.Fatalf("doubling: got %v want 10s", got)
	}
	if got := nextBackoff(40 * time.Second); got != maxPollBackoff {
		t.Fatalf("cap: got %v want %v", got, maxPollBackoff)
	}
}
