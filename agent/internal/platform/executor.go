package platform

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type OSCommandExecutor struct{}

func NewOSCommandExecutor() *OSCommandExecutor {
	return &OSCommandExecutor{}
}

func (e *OSCommandExecutor) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		return text, fmt.Errorf("command %s %v failed: %w", name, args, err)
	}
	return text, nil
}
