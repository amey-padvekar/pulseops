package obs

import (
	"fmt"
	"io"
	"math"
	"sort"
	"sync"
)

// Outcome categories recorded for every /investigate request. timeout and
// validation_fail are sub-categories of fail: a request in either category also
// increments fail_total, so error rate can be computed as fail_total/requests_total.
const (
	OutcomeSuccess        = "success"
	OutcomeFail           = "fail"
	OutcomeTimeout        = "timeout"
	OutcomeValidationFail = "validation_fail"
)

// latencyBucketsMs are the upper bounds (inclusive) of the latency histogram in
// milliseconds. They straddle the 8s p95 alert threshold so histogram_quantile
// has resolution around the alerting boundary.
var latencyBucketsMs = []float64{50, 100, 250, 500, 1000, 2000, 4000, 6000, 8000, 10000}

const reservoirSize = 4096

// Metrics is a thread-safe in-process registry exposing the Phase E2 metric set
// in Prometheus text format. It is intentionally dependency-free so the adapter
// keeps a clean go.mod; Google Managed Prometheus (or any scraper) can read it.
type Metrics struct {
	mu sync.Mutex

	requests       uint64
	success        uint64
	fail           uint64
	timeout        uint64
	validationFail uint64

	bucketCounts []uint64 // cumulative counts, len = len(latencyBucketsMs)+1 (+Inf)
	latencySumMs float64
	latencyCount uint64

	reservoir []float64 // bounded ring of recent latencies for p50/p95
	resNext   int
}

// NewMetrics returns an empty metrics registry.
func NewMetrics() *Metrics {
	return &Metrics{
		bucketCounts: make([]uint64, len(latencyBucketsMs)+1),
		reservoir:    make([]float64, 0, reservoirSize),
	}
}

// Observe records one terminal request outcome and its end-to-end latency.
func (m *Metrics) Observe(outcome string, latencyMs float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requests++
	switch outcome {
	case OutcomeSuccess:
		m.success++
	case OutcomeTimeout:
		m.fail++
		m.timeout++
	case OutcomeValidationFail:
		m.fail++
		m.validationFail++
	default:
		m.fail++
	}

	m.observeLatencyLocked(latencyMs)
}

func (m *Metrics) observeLatencyLocked(v float64) {
	if v < 0 {
		v = 0
	}
	m.latencySumMs += v
	m.latencyCount++

	for i, bound := range latencyBucketsMs {
		if v <= bound {
			m.bucketCounts[i]++
		}
	}
	m.bucketCounts[len(latencyBucketsMs)]++ // +Inf bucket

	if len(m.reservoir) < reservoirSize {
		m.reservoir = append(m.reservoir, v)
		return
	}
	m.reservoir[m.resNext] = v
	m.resNext = (m.resNext + 1) % reservoirSize
}

func (m *Metrics) percentileLocked(p float64) float64 {
	n := len(m.reservoir)
	if n == 0 {
		return 0
	}
	cp := make([]float64, n)
	copy(cp, m.reservoir)
	sort.Float64s(cp)

	idx := int(math.Ceil(p/100*float64(n))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return cp[idx]
}

// Snapshot is a point-in-time copy of the registry, used for tests and the
// controlled-test summary.
type Snapshot struct {
	Requests       uint64
	Success        uint64
	Fail           uint64
	Timeout        uint64
	ValidationFail uint64
	LatencyCount   uint64
	P50Ms          float64
	P95Ms          float64
}

// Snapshot returns a consistent copy of the current counters and percentiles.
func (m *Metrics) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Snapshot{
		Requests:       m.requests,
		Success:        m.success,
		Fail:           m.fail,
		Timeout:        m.timeout,
		ValidationFail: m.validationFail,
		LatencyCount:   m.latencyCount,
		P50Ms:          m.percentileLocked(50),
		P95Ms:          m.percentileLocked(95),
	}
}

// WritePrometheus writes the registry in Prometheus text exposition format.
func (m *Metrics) WritePrometheus(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	writeCounter(w, "investigate_requests_total", "Total investigate requests processed.", m.requests)
	writeCounter(w, "investigate_success_total", "Investigate requests that returned a valid result.", m.success)
	writeCounter(w, "investigate_fail_total", "Investigate requests that failed (includes timeout and validation_fail).", m.fail)
	writeCounter(w, "investigate_timeout_total", "Investigate requests that exceeded the request budget.", m.timeout)
	writeCounter(w, "investigate_validation_fail_total", "Investigate requests rejected by request or model-output validation.", m.validationFail)

	fmt.Fprintf(w, "# HELP investigate_latency_ms Investigation request latency in milliseconds.\n")
	fmt.Fprintf(w, "# TYPE investigate_latency_ms histogram\n")
	for i, bound := range latencyBucketsMs {
		fmt.Fprintf(w, "investigate_latency_ms_bucket{le=\"%g\"} %d\n", bound, m.bucketCounts[i])
	}
	fmt.Fprintf(w, "investigate_latency_ms_bucket{le=\"+Inf\"} %d\n", m.bucketCounts[len(latencyBucketsMs)])
	fmt.Fprintf(w, "investigate_latency_ms_sum %g\n", m.latencySumMs)
	fmt.Fprintf(w, "investigate_latency_ms_count %d\n", m.latencyCount)

	writeGauge(w, "investigate_latency_ms_p50", "Approximate p50 latency over recent requests.", m.percentileLocked(50))
	writeGauge(w, "investigate_latency_ms_p95", "Approximate p95 latency over recent requests.", m.percentileLocked(95))
}

func writeCounter(w io.Writer, name, help string, value uint64) {
	fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	fmt.Fprintf(w, "# TYPE %s counter\n", name)
	fmt.Fprintf(w, "%s %d\n", name, value)
}

func writeGauge(w io.Writer, name, help string, value float64) {
	fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	fmt.Fprintf(w, "# TYPE %s gauge\n", name)
	fmt.Fprintf(w, "%s %g\n", name, value)
}
