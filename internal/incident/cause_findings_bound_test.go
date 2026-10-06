package incident

import (
	"fmt"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// An open incident remembers at most maxCauseFindings root findings, so a
// crash loop of ever-replaced pods cannot grow it for ever.
func TestRememberRootReasonsIsBounded(t *testing.T) {
	p := &Incident{}
	for round := range 4 {
		cause := &rootcause.CauseRecord{}
		for i := range maxCauseFindings {
			cause.RootFindings = append(cause.RootFindings, detection.Finding{
				Entity: inventory.EntityID{Kind: "pod", Namespace: "ns",
					Name: fmt.Sprintf("p%d-%d", round, i)},
				Reason: "CrashLoop",
			})
		}
		p.Cause = cause
		p.rememberRootReasons()
	}
	if got := len(p.causeFindings); got != maxCauseFindings {
		t.Fatalf("remembered %d root findings, want %d", got,
			maxCauseFindings)
	}
}
