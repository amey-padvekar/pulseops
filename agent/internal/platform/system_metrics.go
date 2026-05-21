package platform

import (
	"context"
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
		`$os=Get-CimInstance Win32_OperatingSystem; $cpu=(Get-CimInstance Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average; if (-not $cpu) { $cpu=0 }; $used=((([double]$os.TotalVisibleMemorySize - [double]$os.FreePhysicalMemory) / [double]$os.TotalVisibleMemorySize) * 100); if ($used -lt 0) { $used=0 }; if ($used -gt 100) { $used=100 }; Write-Output (([math]::Round([double]$cpu,2)).ToString() + ',' + ([math]::Round([double]$used,2)).ToString())`,
	)
	if err != nil {
		return SystemMetrics{}, fmt.Errorf("collect windows system metrics: %w", err)
	}

	metrics, parseErr := parseWindowsMetrics(output)
	if parseErr != nil {
		return SystemMetrics{}, parseErr
	}

	return metrics, nil
}

type UnsupportedSystemMetricsCollector struct{}

func (c *UnsupportedSystemMetricsCollector) Collect(context.Context) (SystemMetrics, error) {
	return SystemMetrics{CPUUsage: 0, MemoryUsage: 0}, nil
}

func parseWindowsMetrics(output string) (SystemMetrics, error) {
	line := strings.TrimSpace(output)
	if line == "" {
		return SystemMetrics{}, fmt.Errorf("metrics output is empty")
	}

	parts := strings.Split(line, ",")
	if len(parts) != 2 {
		return SystemMetrics{}, fmt.Errorf("metrics output must be cpu,memory but was %q", line)
	}

	cpu, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return SystemMetrics{}, fmt.Errorf("parse cpu usage: %w", err)
	}
	memory, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return SystemMetrics{}, fmt.Errorf("parse memory usage: %w", err)
	}

	return SystemMetrics{
		CPUUsage:    clampPercent(cpu),
		MemoryUsage: clampPercent(memory),
	}, nil
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
