package platform

import (
	"context"
	"fmt"
	"net"
	"strings"
)

type TCPNetworkChecker struct{}

func NewNetworkChecker() NetworkChecker {
	return &TCPNetworkChecker{}
}

func (c *TCPNetworkChecker) Reachable(ctx context.Context, target string) (bool, error) {
	address := normalizeDialTarget(target)
	dialer := &net.Dialer{}

	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return false, fmt.Errorf("dial %s: %w", address, err)
	}
	_ = conn.Close()

	return true, nil
}

func normalizeDialTarget(target string) string {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "8.8.8.8:53"
	}
	if _, _, err := net.SplitHostPort(trimmed); err == nil {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		return trimmed + ":53"
	}
	if strings.Count(trimmed, ":") > 1 {
		return "[" + trimmed + "]:53"
	}
	return net.JoinHostPort(trimmed, "53")
}
