package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
)

// TestEveryReasonIsHandled fails when a reason is declared in incident
// without a case in sentencesFor.
func TestEveryReasonIsHandled(t *testing.T) {
	for _, reason := range incident.AllReasons {
		if _, known := sentencesFor(reason); !known {
			t.Errorf("reason %q has no case in sentencesFor", reason)
		}
	}
	if _, known := sentencesFor("healthy for 5m0s"); known {
		t.Error("a free-text reason must use the fallback")
	}
}
