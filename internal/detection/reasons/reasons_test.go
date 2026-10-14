package reasons

import "testing"

// Reasons match literally, so the two OOM spellings are distinct values on
// purpose; folding them would drop one source's events.
func TestOOMSpellingsAreDistinct(t *testing.T) {
	if OOMKilled != "OOMKilled" || OOMKILLED != "OOMKILLED" {
		t.Fatalf("OOM spellings changed: %q, %q", OOMKilled, OOMKILLED)
	}
}

func TestDerivedReasons(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"condition", Condition("Ready"), "ConditionFailure.Ready"},
		{"condition empty", Condition(""), "ConditionFailure."},
		{"unknown waiting", UnknownWaiting("SomeNewReason"),
			"UnknownWaiting.SomeNewReason"},
		{"unknown waiting empty", UnknownWaiting(""), UnknownWaitingPrefix},
		{"unusual event", UnusualEvent("SystemOOM"), "UnusualEvent.SystemOOM"},
		{"unusual event empty", UnusualEvent(""), UnusualEventPrefix},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}
