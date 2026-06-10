package incidents

import (
	"strings"
	"testing"
	"time"
)

func TestApprovalAuditLine_IncludesRequiredFields(t *testing.T) {
	approvedAt := time.Date(2026, 5, 23, 22, 10, 0, 0, time.UTC)
	inc := Incident{
		IncidentID:      "INC-1001",
		DeviceID:        "dev-1",
		ApprovedBy:      "demo.operator",
		ApprovedAt:      &approvedAt,
		ApprovedActions: []string{"restart_service", "flush_dns"},
	}

	line := ApprovalAuditLine(inc)

	wants := []string{
		"approval_audit",
		"incident_id=INC-1001",
		"device_id=dev-1",
		"approved_by=demo.operator",
		"approved_at=2026-05-23T22:10:00Z",
		"approved_actions=restart_service,flush_dns",
	}
	for _, want := range wants {
		if !strings.Contains(line, want) {
			t.Fatalf("audit line missing %q\ngot: %s", want, line)
		}
	}
}

func TestApprovalAuditLine_NormalizesTimestampToUTC(t *testing.T) {
	// A non-UTC timestamp must be rendered in ISO 8601 UTC (trailing Z), not its
	// original offset, so audit evidence is unambiguous.
	ist := time.FixedZone("IST", 5*3600+30*60)
	approvedAt := time.Date(2026, 5, 24, 3, 40, 0, 0, ist) // == 2026-05-23T22:10:00Z
	inc := Incident{
		IncidentID:      "INC-1001",
		DeviceID:        "dev-1",
		ApprovedBy:      "demo.operator",
		ApprovedAt:      &approvedAt,
		ApprovedActions: []string{"restart_service"},
	}

	line := ApprovalAuditLine(inc)
	if !strings.Contains(line, "approved_at=2026-05-23T22:10:00Z") {
		t.Fatalf("expected UTC-normalized timestamp, got: %s", line)
	}
}

func TestApprovalAuditLine_MissingTimestampRendersEmpty(t *testing.T) {
	inc := Incident{
		IncidentID:      "INC-1001",
		DeviceID:        "dev-1",
		ApprovedBy:      "demo.operator",
		ApprovedActions: []string{"restart_service"},
	}

	line := ApprovalAuditLine(inc)
	// The field is present but empty — never a misleading zero time.
	if strings.Contains(line, "approved_at=0001-01-01") {
		t.Fatalf("missing timestamp should not render zero time, got: %s", line)
	}
	if !strings.Contains(line, "approved_at= ") {
		t.Fatalf("expected empty approved_at field, got: %s", line)
	}
}
