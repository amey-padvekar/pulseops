package agentbuilder

import (
	"fmt"
	"strings"

	"github.com/certainelf/pulseops/backend/internal/incidents"
)

// defaultMaxLogEvidence caps how many log snippets enter the evidence set, so raw logs
// never dominate or flood the summary request (step 4.7 task 2).
const defaultMaxLogEvidence = 3

// selectEvidence builds the bounded, high-value evidence set for a final summary
// (Phase 11 step 4.7). It draws a small number of concise, compacted facts from each
// stage of the incident lifecycle — initial unhealthy telemetry, log snippets, the AI
// investigation result, approved actions, execution results, and validation telemetry —
// in a deterministic most-explanatory-first order.
//
// The closing outcome line (recovery confirmed, or the failure reason) is always retained
// even when the cap is hit, so the evidence always reflects whether the incident resolved
// or failed (task 4). Every entry is whitespace-collapsed and length-bounded (task 3), and
// log snippets are capped (task 2) so the set stays grounded without becoming noisy.
func selectEvidence(incident incidents.Incident, recentLogs []string, maxEvidence, maxSnippet int) []string {
	c := evidenceCollector{maxSnippet: maxSnippet}

	// 1. Initial unhealthy telemetry — the detection reason and the observed status that
	//    opened the incident.
	c.add("Detection: " + strings.TrimSpace(incident.Reason))
	if st := strings.TrimSpace(incident.ServiceStatus); st != "" {
		c.add("Initial telemetry: serviceStatus=" + st)
	}

	// 2. AI investigation result (concise, not the full investigation summary).
	if cause := strings.TrimSpace(incident.ProbableCause); cause != "" {
		if incident.Confidence > 0 {
			c.add(fmt.Sprintf("Diagnosis: %s (confidence %.2f)", cause, incident.Confidence))
		} else {
			c.add("Diagnosis: " + cause)
		}
	}

	// 3. Relevant log snippets — most recent first, hard-capped so raw logs never flood.
	for _, line := range compactLogEvidence(recentLogs, defaultMaxLogEvidence, maxSnippet) {
		c.add("Log: " + line)
	}

	// 4. Approved remediation actions.
	if approved := trimmedNonEmpty(incident.ApprovedActions); len(approved) > 0 {
		c.add("Approved remediation: " + strings.Join(approved, ", "))
	}

	// 5. Execution results — one concise fact per executed action, else the overall status.
	if len(incident.RemediationResults) > 0 {
		for _, r := range incident.RemediationResults {
			id := strings.TrimSpace(r.ActionID)
			if id == "" {
				continue
			}
			c.add(fmt.Sprintf("Execution: %s %s", id, orPlaceholder(r.Status, "status unknown")))
		}
	} else if rs := strings.TrimSpace(incident.RemediationStatus); rs != "" {
		c.add("Execution status: " + rs)
	}

	// 6. Validation telemetry — the strongest recovery/failure signal.
	if snap := incident.LastValidationSnapshot; snap != nil {
		c.add(fmt.Sprintf(
			"Validation telemetry: serviceStatus=%s heartbeat=%t networkReachable=%t (%s)",
			snap.ServiceStatus, snap.Heartbeat, snap.NetworkReachable, strings.TrimSpace(snap.Reason),
		))
	} else if r := strings.TrimSpace(incident.LastValidationReason); r != "" {
		c.add("Validation: " + r)
	}

	// 7. Outcome line (task 4) — always retained against the cap below.
	outcome := outcomeEvidenceLine(incident)

	return c.finalize(outcome, maxEvidence)
}

// evidenceCollector accumulates compacted, non-empty evidence lines.
type evidenceCollector struct {
	lines      []string
	maxSnippet int
}

// add compacts and bounds s, then appends it when non-empty. A line whose payload is just
// a label with no content (e.g. "Detection: ") is dropped.
func (c *evidenceCollector) add(s string) {
	s = truncateSnippet(collapseWhitespace(strings.TrimSpace(s)), c.maxSnippet)
	if s == "" || strings.HasSuffix(s, ":") {
		return
	}
	c.lines = append(c.lines, s)
}

// finalize caps the collected lines to maxEvidence while guaranteeing the outcome line is
// present (it takes the last slot when the cap would otherwise drop it).
func (c *evidenceCollector) finalize(outcome string, maxEvidence int) []string {
	out := c.lines
	outcome = truncateSnippet(collapseWhitespace(strings.TrimSpace(outcome)), c.maxSnippet)

	if outcome == "" {
		if len(out) > maxEvidence {
			out = out[:maxEvidence]
		}
		return out
	}

	if len(out) >= maxEvidence {
		out = append([]string{}, out[:maxEvidence-1]...)
	}
	return append(out, outcome)
}

// outcomeEvidenceLine returns a single line that states whether the incident resolved or
// failed. For a failure it surfaces the recorded failure reason verbatim so the evidence
// explains why recovery was never confirmed.
func outcomeEvidenceLine(incident incidents.Incident) string {
	switch incident.State {
	case incidents.StateResolved:
		if incident.HealthyCycleCount > 0 && incident.RequiredHealthyCycles > 0 {
			return fmt.Sprintf(
				"Outcome: resolved - recovery confirmed after %d/%d healthy cycles",
				incident.HealthyCycleCount, incident.RequiredHealthyCycles,
			)
		}
		return "Outcome: resolved - service health recovered"
	case incidents.StateFailed:
		if r := strings.TrimSpace(incident.ValidationFailureReason); r != "" {
			return "Validation failure: " + r
		}
		return "Outcome: failed - recovery was not confirmed"
	default:
		return ""
	}
}

// compactLogEvidence returns at most maxLogs of the most recent non-empty log lines, each
// whitespace-collapsed and length-bounded. This keeps relevant log context without ever
// dumping the full raw log stream into the summary request (task 2).
func compactLogEvidence(logs []string, maxLogs, maxSnippet int) []string {
	if len(logs) == 0 || maxLogs <= 0 {
		return nil
	}

	clean := make([]string, 0, len(logs))
	for _, line := range logs {
		t := truncateSnippet(collapseWhitespace(strings.TrimSpace(line)), maxSnippet)
		if t != "" {
			clean = append(clean, t)
		}
	}

	if len(clean) <= maxLogs {
		return clean
	}
	// Keep the most recent lines (tail), matching the investigation packer's log handling.
	return append([]string{}, clean[len(clean)-maxLogs:]...)
}
