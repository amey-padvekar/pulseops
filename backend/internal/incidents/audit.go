package incidents

import (
	"fmt"
	"strings"
	"time"
)

// ApprovalAuditLine returns a single structured log line that proves who approved
// which actions on an incident and when. It is the canonical approval-evidence
// format for logs and demo artifacts (Phase 8 task 4.8).
//
// Timestamps are ISO 8601 / RFC 3339 in UTC; a missing ApprovedAt renders as an
// empty value rather than a misleading zero time. Approved action IDs are emitted
// as a comma-separated list so the exact approved set is recoverable from logs.
func ApprovalAuditLine(inc Incident) string {
	approvedAt := ""
	if inc.ApprovedAt != nil {
		approvedAt = inc.ApprovedAt.UTC().Format(time.RFC3339)
	}

	return fmt.Sprintf(
		"approval_audit incident_id=%s device_id=%s approved_by=%s approved_at=%s approved_actions=%s",
		inc.IncidentID,
		inc.DeviceID,
		inc.ApprovedBy,
		approvedAt,
		strings.Join(inc.ApprovedActions, ","),
	)
}
