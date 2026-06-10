package remediation

import "sort"

// Backend remediation catalog: the authoritative set of action IDs that may ever be
// approved and executed. The AI may only recommend these (mirrored by the agentbuilder
// parser whitelist), and the approval/execution path must never act on an action ID
// outside this set. Keeping the catalog here makes the remediation/execution layer the
// single source of truth for what is runnable.
const (
	ActionRestartService = "restart_service"
	ActionFlushDNS       = "flush_dns"
	ActionReconnectVPN   = "reconnect_vpn"
)

var approvedActionIDs = map[string]struct{}{
	ActionRestartService: {},
	ActionFlushDNS:       {},
	ActionReconnectVPN:   {},
}

// IsApprovedAction reports whether id is part of the backend remediation catalog.
func IsApprovedAction(id string) bool {
	_, ok := approvedActionIDs[id]
	return ok
}

// ApprovedActionIDs returns the catalog action IDs in sorted order. The returned
// slice is a fresh copy, safe for the caller to mutate.
func ApprovedActionIDs() []string {
	ids := make([]string, 0, len(approvedActionIDs))
	for id := range approvedActionIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
