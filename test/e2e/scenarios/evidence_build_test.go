//go:build e2e

package scenarios

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/test/e2e/harness"
)

// evidenceWait bounds how long a scenario waits for a message that
// carries the gathered evidence; a re-investigation can bring it with
// the first update.
const evidenceWait = 6 * time.Minute

// ExpectGatheredEvidence waits for a Pro message about resource that
// carries the investigation's checks, then requires that it asks for no
// read-only command, states a confirmed cause and that at least one
// check read something. namespace is empty for a cluster-scoped root.
func (s *Scenario) ExpectGatheredEvidence(namespace, resource string) {
	s.T.Helper()
	ctx, cancel := context.WithTimeout(s.Ctx, evidenceWait)
	defer cancel()
	match := harness.AuditMatch{Namespace: namespace, Resource: resource}
	var seen []string
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		entries, err := s.Env.Audit.Matching(ctx, match)
		if err == nil {
			seen = seen[:0]
			for _, e := range entries {
				// A roll-up or digest carries the incident's own message,
				// so its entry counts the same as a message of its own.
				if gathered(e) {
					return
				}
				seen = append(seen, describeEntry(e))
			}
		}
		select {
		case <-ctx.Done():
			s.T.Fatalf("no message about %s with gathered evidence; "+
				"entries: %v", resource, seen)
		case <-ticker.C:
		}
	}
}

// gathered reports a message that runs every read itself and proves
// its cause with at least one check that read something.
func gathered(e harness.AuditEntry) bool {
	ok := false
	for _, check := range e.Evidence {
		ok = ok || strings.HasSuffix(check, ":ok")
	}
	return ok && e.ReadSteps == 0 && e.Confidence == "confirmed"
}

// describeEntry is what a failure shows of one audit entry.
func describeEntry(e harness.AuditEntry) string {
	return fmt.Sprintf("{action %s delivery %q confidence %q readSteps %d "+
		"evidence %v}", e.Action, e.Delivery, e.Confidence, e.ReadSteps,
		e.Evidence)
}
