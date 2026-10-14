package reasons

import "testing"

// Reasons match literally, so the two OOM spellings are distinct values on
// purpose; folding them would drop one source's events.
func TestOOMSpellingsAreDistinct(t *testing.T) {
	if OOMKilled != "OOMKilled" || OOMKILLED != "OOMKILLED" {
		t.Fatalf("OOM spellings changed: %q, %q", OOMKilled, OOMKILLED)
	}
}
