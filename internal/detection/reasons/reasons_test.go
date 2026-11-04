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

// Only an error exit of the program itself is a crash: a stop by
// signal and a memory kill are not.
func TestCrashExitLeavesOutKillsAndOOM(t *testing.T) {
	cases := []struct {
		code   int
		reason string
		want   bool
	}{
		{1, Error, true}, {2, "", true}, {0, Completed, false},
		{137, Error, false}, {143, Error, false},
		{1, OOMKilled, false}, {1, OOMKILLED, false},
	}
	for _, c := range cases {
		if got := CrashExit(c.code, c.reason); got != c.want {
			t.Errorf("CrashExit(%d, %q) = %v", c.code, c.reason, got)
		}
	}
}
