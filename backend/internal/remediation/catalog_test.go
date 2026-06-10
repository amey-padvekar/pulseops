package remediation

import (
	"reflect"
	"testing"
)

func TestIsApprovedAction(t *testing.T) {
	approved := []string{ActionRestartService, ActionFlushDNS, ActionReconnectVPN}
	for _, id := range approved {
		if !IsApprovedAction(id) {
			t.Errorf("expected %q to be an approved catalog action", id)
		}
	}

	for _, id := range []string{"", "shutdown", "rm -rf /", "restart_service ", "RESTART_SERVICE"} {
		if IsApprovedAction(id) {
			t.Errorf("did not expect %q to be an approved catalog action", id)
		}
	}
}

func TestApprovedActionIDs_SortedCopy(t *testing.T) {
	got := ApprovedActionIDs()
	want := []string{"flush_dns", "reconnect_vpn", "restart_service"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ApprovedActionIDs: got %v want %v", got, want)
	}

	// Mutating the returned slice must not affect the catalog.
	got[0] = "tampered"
	if !IsApprovedAction("flush_dns") {
		t.Fatal("catalog was mutated through the returned slice")
	}
}
