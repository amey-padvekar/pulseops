package platform

import (
	"context"
	"fmt"
	"runtime"
	"strings"
)

func NewServiceChecker(executor CommandExecutor) ServiceChecker {
	if runtime.GOOS == "windows" {
		return &WindowsServiceChecker{executor: executor}
	}
	return &UnsupportedServiceChecker{}
}

type WindowsServiceChecker struct {
	executor CommandExecutor
}

func (c *WindowsServiceChecker) CheckService(ctx context.Context, serviceName string) (string, error) {
	output, err := c.executor.Run(ctx, "sc.exe", "query", serviceName)
	if err != nil {
		if strings.Contains(strings.ToLower(output), "does not exist") {
			return ServiceStateUnknown, nil
		}
		return ServiceStateUnknown, fmt.Errorf("query windows service %q: %w", serviceName, err)
	}

	return mapWindowsSCState(output), nil
}

type UnsupportedServiceChecker struct{}

func (c *UnsupportedServiceChecker) CheckService(context.Context, string) (string, error) {
	return ServiceStateUnknown, nil
}

func mapWindowsSCState(output string) string {
	upper := strings.ToUpper(output)

	switch {
	case strings.Contains(upper, " RUNNING"):
		return ServiceStateRunning
	case strings.Contains(upper, " STOPPED"):
		return ServiceStateStopped
	case strings.Contains(upper, " START_PENDING"):
		return ServiceStateDegraded
	case strings.Contains(upper, " STOP_PENDING"):
		return ServiceStateDegraded
	case strings.Contains(upper, " PAUSED"):
		return ServiceStateDegraded
	default:
		return ServiceStateUnknown
	}
}
