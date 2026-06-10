package remediation

import (
	"context"
	"errors"
	"sort"

	"github.com/certainelf/pulseops/agent/internal/platform"
)

// Agent-side remediation whitelist (Phase 9 step 4.5). These IDs mirror the backend
// catalog (backend/internal/remediation/catalog.go). The agent enforces the whitelist
// independently so it never executes an action the backend did not vet, even if a
// malformed or unexpected payload reaches it.
const (
	ActionRestartService = "restart_service"
	ActionFlushDNS       = "flush_dns"
	ActionReconnectVPN   = "reconnect_vpn"
)

var whitelistedActions = map[string]struct{}{
	ActionRestartService: {},
	ActionFlushDNS:       {},
	ActionReconnectVPN:   {},
}

// IsWhitelisted reports whether id is an action the agent is allowed to execute.
func IsWhitelisted(id string) bool {
	_, ok := whitelistedActions[id]
	return ok
}

// WhitelistedActionIDs returns the whitelist in sorted order (fresh copy).
func WhitelistedActionIDs() []string {
	ids := make([]string, 0, len(whitelistedActions))
	for id := range whitelistedActions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Mapper errors.
var (
	// ErrActionNotWhitelisted means the action ID is not one the agent will execute.
	// Callers should report this as a "rejected" execution outcome (never run anything).
	ErrActionNotWhitelisted = errors.New("remediation action is not whitelisted")
	// ErrMissingTarget means a target-requiring action arrived without a target.
	ErrMissingTarget = errors.New("remediation action requires a target")
)

// Mapper converts whitelisted action IDs into concrete, bounded platform operations
// and executes them. It is the only place that decides which platform method an action
// maps to; it never executes arbitrary text. Unknown actions are rejected before any
// platform call (step 4.5 task 4).
type Mapper struct {
	performer platform.Remediator
}

// NewMapper builds a Mapper over a platform Remediator (typically platform.NewRemediator).
func NewMapper(performer platform.Remediator) *Mapper {
	return &Mapper{performer: performer}
}

// Execute runs the platform operation mapped from action and returns its structured
// CommandResult. It rejects non-whitelisted actions and target-requiring actions that
// arrived without a target, in both cases without invoking any platform command (the
// returned CommandResult is the zero value).
func (m *Mapper) Execute(ctx context.Context, action Action) (platform.CommandResult, error) {
	if !IsWhitelisted(action.ActionID) {
		return platform.CommandResult{}, ErrActionNotWhitelisted
	}

	switch action.ActionID {
	case ActionRestartService:
		if action.Target == "" {
			return platform.CommandResult{}, ErrMissingTarget
		}
		return m.performer.RestartService(ctx, action.Target)
	case ActionFlushDNS:
		return m.performer.FlushDNS(ctx)
	case ActionReconnectVPN:
		if action.Target == "" {
			return platform.CommandResult{}, ErrMissingTarget
		}
		return m.performer.ReconnectVPN(ctx, action.Target)
	default:
		// Defensive: the whitelist and this switch must stay in sync. If an action is
		// whitelisted but unmapped, reject rather than silently no-op.
		return platform.CommandResult{}, ErrActionNotWhitelisted
	}
}
