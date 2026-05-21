package platform

import "testing"

func TestNormalizeDialTarget(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{name: "empty defaults", target: "", want: "8.8.8.8:53"},
		{name: "host only", target: "1.1.1.1", want: "1.1.1.1:53"},
		{name: "host and port", target: "example.com:443", want: "example.com:443"},
		{name: "ipv6 host", target: "2001:4860:4860::8888", want: "[2001:4860:4860::8888]:53"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeDialTarget(tt.target)
			if got != tt.want {
				t.Fatalf("normalizeDialTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}
