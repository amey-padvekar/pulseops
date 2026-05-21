package platform

import (
	"strings"
	"testing"
)

func TestNormalizeRecentLogLimit(t *testing.T) {
	if got := normalizeRecentLogLimit(0); got != defaultRecentLogLimit {
		t.Fatalf("normalizeRecentLogLimit(0) = %d, want %d", got, defaultRecentLogLimit)
	}
	if got := normalizeRecentLogLimit(500); got != 100 {
		t.Fatalf("normalizeRecentLogLimit(500) = %d, want %d", got, 100)
	}
	if got := normalizeRecentLogLimit(12); got != 12 {
		t.Fatalf("normalizeRecentLogLimit(12) = %d, want %d", got, 12)
	}
}

func TestNormalizeLogLine(t *testing.T) {
	if got := normalizeLogLine("   one\n\ttwo   three   "); got != "one two three" {
		t.Fatalf("normalizeLogLine() = %q, want %q", got, "one two three")
	}

	tooLong := strings.Repeat("a", 2100)
	if got := normalizeLogLine(tooLong); len(got) != 2048 {
		t.Fatalf("normalizeLogLine() length = %d, want %d", len(got), 2048)
	}
}
