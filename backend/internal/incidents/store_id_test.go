package incidents

import (
	"strconv"
	"testing"
)

// TestCreateOrGetActiveAssignsUniqueIDs guards against same-tick incident ID collisions:
// rapid creates with distinct dedupe keys must each receive a unique ID even when the clock
// does not advance between them (coarse Windows timer). Before the fix, colliding timestamp
// IDs overwrote each other in byID and silently dropped incidents.
func TestCreateOrGetActiveAssignsUniqueIDs(t *testing.T) {
	s := NewStore()
	seen := make(map[string]bool)
	const n = 50

	for i := 0; i < n; i++ {
		dev := "dev-" + strconv.Itoa(i)
		inc, created := s.CreateOrGetActive(dev+"|svc|sig", NewIncident("", dev, "svc", "stopped", SeverityMedium, "reason"))
		if !created {
			t.Fatalf("iteration %d: created=false, want true", i)
		}
		if seen[inc.IncidentID] {
			t.Fatalf("duplicate incident ID %q at iteration %d", inc.IncidentID, i)
		}
		seen[inc.IncidentID] = true
	}

	if len(seen) != n {
		t.Fatalf("unique IDs = %d, want %d", len(seen), n)
	}
}
