package obs

import (
	"bytes"
	"strings"
	"testing"
)

func TestObserveCountsOutcomes(t *testing.T) {
	m := NewMetrics()
	m.Observe(OutcomeSuccess, 100)
	m.Observe(OutcomeSuccess, 200)
	m.Observe(OutcomeValidationFail, 5)
	m.Observe(OutcomeTimeout, 10000)
	m.Observe(OutcomeFail, 9000)

	s := m.Snapshot()
	if s.Requests != 5 {
		t.Fatalf("requests = %d, want 5", s.Requests)
	}
	if s.Success != 2 {
		t.Fatalf("success = %d, want 2", s.Success)
	}
	// fail_total is the umbrella: validation_fail + timeout + plain fail all count.
	if s.Fail != 3 {
		t.Fatalf("fail = %d, want 3", s.Fail)
	}
	if s.Timeout != 1 {
		t.Fatalf("timeout = %d, want 1", s.Timeout)
	}
	if s.ValidationFail != 1 {
		t.Fatalf("validationFail = %d, want 1", s.ValidationFail)
	}
}

// TestP95CrossesAlertThreshold proves the percentile math used by the
// "p95 latency > 8s" alert: a population dominated by slow requests pushes p95
// above the 8000ms boundary.
func TestP95CrossesAlertThreshold(t *testing.T) {
	m := NewMetrics()
	for i := 0; i < 90; i++ {
		m.Observe(OutcomeSuccess, 9000)
	}
	for i := 0; i < 10; i++ {
		m.Observe(OutcomeSuccess, 200)
	}

	s := m.Snapshot()
	if s.P95Ms <= 8000 {
		t.Fatalf("p95 = %v, want > 8000", s.P95Ms)
	}
}

func TestP95BelowThresholdWhenFast(t *testing.T) {
	m := NewMetrics()
	for i := 0; i < 100; i++ {
		m.Observe(OutcomeSuccess, 250)
	}
	s := m.Snapshot()
	if s.P95Ms > 8000 {
		t.Fatalf("p95 = %v, want <= 8000", s.P95Ms)
	}
}

func TestWritePrometheusExposesRequiredSeries(t *testing.T) {
	m := NewMetrics()
	m.Observe(OutcomeSuccess, 120)
	m.Observe(OutcomeValidationFail, 5)

	var buf bytes.Buffer
	m.WritePrometheus(&buf)
	out := buf.String()

	required := []string{
		"investigate_requests_total 2",
		"investigate_success_total 1",
		"investigate_fail_total 1",
		"investigate_timeout_total 0",
		"investigate_validation_fail_total 1",
		"investigate_latency_ms_bucket{le=\"8000\"}",
		"investigate_latency_ms_bucket{le=\"+Inf\"} 2",
		"investigate_latency_ms_count 2",
		"investigate_latency_ms_p50",
		"investigate_latency_ms_p95",
		"# TYPE investigate_requests_total counter",
		"# TYPE investigate_latency_ms histogram",
	}
	for _, want := range required {
		if !strings.Contains(out, want) {
			t.Fatalf("metrics output missing %q\n---\n%s", want, out)
		}
	}
}

func TestHistogramBucketsAreCumulative(t *testing.T) {
	m := NewMetrics()
	m.Observe(OutcomeSuccess, 40)   // <= 50
	m.Observe(OutcomeSuccess, 4500) // <= 6000

	var buf bytes.Buffer
	m.WritePrometheus(&buf)
	out := buf.String()

	// The 40ms sample falls in every bucket >= 50; the 4500ms sample joins from
	// the 6000 bucket onward. So le="6000" must contain both observations.
	if !strings.Contains(out, "investigate_latency_ms_bucket{le=\"50\"} 1") {
		t.Fatalf("le=50 bucket should be 1\n%s", out)
	}
	if !strings.Contains(out, "investigate_latency_ms_bucket{le=\"6000\"} 2") {
		t.Fatalf("le=6000 bucket should be cumulative 2\n%s", out)
	}
}
