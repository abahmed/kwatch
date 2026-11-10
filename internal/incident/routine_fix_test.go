package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// nightlyNodeLoss is a lost node at the same time three nights running,
// each earlier night ended by fix.
func nightlyNodeLoss(fix Fix) *Incident {
	node := entity(kube.KindNode, "n1")
	node.Namespace = ""
	f := sig(node, reasons.NodeNotReady, detection.Critical)
	night := func(d int) time.Time {
		return t0.Add(-time.Duration(d) * 24 * time.Hour)
	}
	return &Incident{Root: node, Opened: t0,
		Occurrences: []time.Time{night(2), night(1), t0},
		Members:     map[detection.Key]detection.Finding{f.Key(): f},
		History: []Occurrence{{Opened: night(2), Fix: fix},
			{Opened: night(1), Fix: fix}}}
}

// A repeat someone fixed each time (a rollback) is a repeated outage,
// not a routine: it pages as on its first night.
func TestFixedRepeatsAreNoRoutine(t *testing.T) {
	if tier, why := tierWhy(nightlyNodeLoss(FixRollback)); tier != Page {
		t.Fatalf("tier %v (%s), want page", tier, why)
	}
}

// A routine a page rule names still notifies: a lost node is never only
// a digest line, even when it goes away on its own each night.
func TestRoutineNeverHidesAPageRule(t *testing.T) {
	if tier, why := tierWhy(nightlyNodeLoss(FixNone)); tier < Notify {
		t.Fatalf("tier %v (%s), want at least notify", tier, why)
	}
}
