package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

func NewSystemMetricsCollector(executor CommandExecutor) SystemMetricsCollector {
	if runtime.GOOS == "windows" {
		return &WindowsSystemMetricsCollector{executor: executor}
	}
	return &UnsupportedSystemMetricsCollector{}
}

type WindowsSystemMetricsCollector struct {
	executor CommandExecutor
}

func (c *WindowsSystemMetricsCollector) Collect(ctx context.Context) (SystemMetrics, error) {
	output, err := c.executor.Run(
		ctx,
		"powershell",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		`$os=Get-CimInstance Win32_OperatingSystem; $cpu=(Get-CimInstance Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average; if (-not $cpu) { $cpu=0 }; $used=((([double]$os.TotalVisibleMemorySize - [double]$os.FreePhysicalMemory) / [double]$os.TotalVisibleMemorySize) * 100); if ($used -lt 0) { $used=0 }; if ($used -gt 100) { $used=100 }; $result=@{cpu=[math]::Round([double]$cpu,2); memory=[math]::Round([double]$used,2)}; $result | ConvertTo-Json -Compress`,
	)
	if err != nil {
		return SystemMetrics{}, fmt.Errorf("collect windows system metrics: %w", err)
	}

	metrics, parseErr := parseWindowsMetricsOutput(output)
	if parseErr != nil {
		return SystemMetrics{}, parseErr
	}

	return metrics, nil
}

type UnsupportedSystemMetricsCollector struct{}

func (c *UnsupportedSystemMetricsCollector) Collect(context.Context) (SystemMetrics, error) {
	return SystemMetrics{CPUUsage: 0, MemoryUsage: 0}, nil
}

type windowsMetricsPayload struct {
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
}

func parseWindowsMetricsOutput(output string) (SystemMetrics, error) {
	line := strings.TrimSpace(output)
	if line == "" {
		return SystemMetrics{}, fmt.Errorf("metrics output is empty")
	}

	if strings.HasPrefix(line, "{") {
		var payload windowsMetricsPayload
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			return SystemMetrics{}, fmt.Errorf("parse metrics json: %w", err)
		}
		return SystemMetrics{
			CPUUsage:    clampPercent(payload.CPU),
			MemoryUsage: clampPercent(payload.Memory),
		}, nil
	}

	// Backward compatibility for prior csv-style output.
	return parseWindowsMetrics(line)
}

func parseWindowsMetrics(output string) (SystemMetrics, error) {
	line := strings.TrimSpace(output)
	if line == "" {
		return SystemMetrics{}, fmt.Errorf("metrics output is empty")
	}

	delimiter := ","
	if strings.Contains(line, ";") {
		delimiter = ";"
	}

	parts := strings.Split(line, delimiter)
	if len(parts) != 2 {
		return SystemMetrics{}, fmt.Errorf("metrics output must be cpu,memory but was %q", line)
	}

	cpu, err := parsePercent(parts[0])
	if err != nil {
		return SystemMetrics{}, fmt.Errorf("parse cpu usage: %w", err)
	}
	memory, err := parsePercent(parts[1])
	if err != nil {
		return SystemMetrics{}, fmt.Errorf("parse memory usage: %w", err)
	}

	return SystemMetrics{
		CPUUsage:    clampPercent(cpu),
		MemoryUsage: clampPercent(memory),
	}, nil
}

func parsePercent(raw string) (float64, error) {
	value := strings.TrimSpace(raw)
	if strings.Contains(value, ",") && !strings.Contains(value, ".") {
		value = strings.ReplaceAll(value, ",", ".")
	}
	return strconv.ParseFloat(value, 64)
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
