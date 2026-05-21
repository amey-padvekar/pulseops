package platform

import "testing"

func TestMapWindowsSCState(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "running", output: "STATE              : 4  RUNNING", want: ServiceStateRunning},
		{name: "stopped", output: "STATE              : 1  STOPPED", want: ServiceStateStopped},
		{name: "start pending", output: "STATE              : 2  START_PENDING", want: ServiceStateDegraded},
		{name: "paused", output: "STATE              : 7  PAUSED", want: ServiceStateDegraded},
		{name: "unknown", output: "STATE              : 9  OTHER_STATE", want: ServiceStateUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapWindowsSCState(tt.output)
			if got != tt.want {
				t.Fatalf("mapWindowsSCState() = %q, want %q", got, tt.want)
			}
		})
	}
}
