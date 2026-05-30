package platform

import "testing"

func TestParseWindowsMetrics(t *testing.T) {
	metrics, err := parseWindowsMetrics("23.5,61.2")
	if err != nil {
		t.Fatalf("parseWindowsMetrics() error = %v", err)
	}
	if metrics.CPUUsage != 23.5 {
		t.Fatalf("CPUUsage = %v, want %v", metrics.CPUUsage, 23.5)
	}
	if metrics.MemoryUsage != 61.2 {
		t.Fatalf("MemoryUsage = %v, want %v", metrics.MemoryUsage, 61.2)
	}
}

func TestParseWindowsMetricsClampsRange(t *testing.T) {
	metrics, err := parseWindowsMetrics("-4,103")
	if err != nil {
		t.Fatalf("parseWindowsMetrics() error = %v", err)
	}
	if metrics.CPUUsage != 0 {
		t.Fatalf("CPUUsage = %v, want %v", metrics.CPUUsage, 0)
	}
	if metrics.MemoryUsage != 100 {
		t.Fatalf("MemoryUsage = %v, want %v", metrics.MemoryUsage, 100)
	}
}

func TestParseWindowsMetrics_SemicolonDelimiterAndDecimalComma(t *testing.T) {
	metrics, err := parseWindowsMetrics("23,5;61,2")
	if err != nil {
		t.Fatalf("parseWindowsMetrics() error = %v", err)
	}
	if metrics.CPUUsage != 23.5 {
		t.Fatalf("CPUUsage = %v, want %v", metrics.CPUUsage, 23.5)
	}
	if metrics.MemoryUsage != 61.2 {
		t.Fatalf("MemoryUsage = %v, want %v", metrics.MemoryUsage, 61.2)
	}
}

func TestParseWindowsMetricsOutput_JSON(t *testing.T) {
	metrics, err := parseWindowsMetricsOutput(`{"cpu":12.34,"memory":56.78}`)
	if err != nil {
		t.Fatalf("parseWindowsMetricsOutput() error = %v", err)
	}
	if metrics.CPUUsage != 12.34 {
		t.Fatalf("CPUUsage = %v, want %v", metrics.CPUUsage, 12.34)
	}
	if metrics.MemoryUsage != 56.78 {
		t.Fatalf("MemoryUsage = %v, want %v", metrics.MemoryUsage, 56.78)
	}
}
