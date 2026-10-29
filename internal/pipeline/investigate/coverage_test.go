package investigate

import (
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
)

// TestChecksCoverEveryStepCheck fails when a check compose relies on to
// drop a read-only step is not registered: the step would be dropped
// for a read nobody makes.
func TestChecksCoverEveryStepCheck(t *testing.T) {
	registered := map[string]bool{}
	for _, c := range checks {
		if registered[c.name] {
			t.Errorf("check %q is registered twice", c.name)
		}
		registered[c.name] = true
	}
	for _, name := range incident.StepChecks {
		if !registered[name] {
			t.Errorf("step check %q has no registered check", name)
		}
	}
}
