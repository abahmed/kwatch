package detection

import "testing"

// The memo of normalised lines is state, so every tracker owns its own:
// two engines in one process, or two tests, never see each other's lines.
func TestTrackersDoNotShareASignatureMemo(t *testing.T) {
	first, second := NewTracker(), NewTracker()

	if first.signer == nil || first.signer == second.signer {
		t.Fatal("each tracker must own a Signer of its own")
	}
}
