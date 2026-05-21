package platform

import (
	"context"
	"fmt"
	"runtime"
	"strings"
)

const defaultRecentLogLimit = 10

func NewLogCollector(executor CommandExecutor) LogCollector {
	if runtime.GOOS == "windows" {
		return &WindowsEventLogCollector{executor: executor}
	}
	return &NoopLogCollector{}
}

type WindowsEventLogCollector struct {
	executor CommandExecutor
}

func (c *WindowsEventLogCollector) CollectRecent(ctx context.Context, serviceName string, limit int) ([]string, error) {
	limit = normalizeRecentLogLimit(limit)
	escapedService := escapePowerShellSingleQuoted(serviceName)

	script := fmt.Sprintf(
		`$service='%s'; $limit=%d; $events=Get-WinEvent -LogName System -MaxEvents 200 -ErrorAction SilentlyContinue | Where-Object { $_.Message -like "*${service}*" -or $_.ProviderName -like "*${service}*" } | Select-Object -First $limit; if (-not $events) { return }; $events | ForEach-Object { $msg=($_.Message -replace "[\r\n]+", " "); "{0}|{1}|{2}|{3}|{4}" -f $_.TimeCreated.ToUniversalTime().ToString("o"), $_.ProviderName, $_.Id, $_.LevelDisplayName, $msg }`,
		escapedService,
		limit,
	)

	output, err := c.executor.Run(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return nil, fmt.Errorf("collect windows event logs for service %q: %w", serviceName, err)
	}

	lines := strings.Split(output, "\n")
	logs := make([]string, 0, limit)
	for _, line := range lines {
		normalized := normalizeLogLine(line)
		if normalized == "" {
			continue
		}
		logs = append(logs, normalized)
		if len(logs) >= limit {
			break
		}
	}

	return logs, nil
}

type NoopLogCollector struct{}

func (c *NoopLogCollector) CollectRecent(context.Context, string, int) ([]string, error) {
	return []string{}, nil
}

func normalizeRecentLogLimit(limit int) int {
	if limit <= 0 {
		return defaultRecentLogLimit
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func normalizeLogLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	normalized := strings.Join(strings.Fields(trimmed), " ")
	if len(normalized) > 2048 {
		return normalized[:2048]
	}
	return normalized
}

func escapePowerShellSingleQuoted(input string) string {
	return strings.ReplaceAll(input, "'", "''")
}
