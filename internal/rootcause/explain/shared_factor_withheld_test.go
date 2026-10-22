package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// threeOnSharedNode fails three workloads on one node with nothing else to
// explain them, so a shared factor is the cause. It returns a failure.
func threeOnSharedNode(f *fixture) inventory.EntityID {
	var first inventory.EntityID
	for i := 0; i < 3; i++ {
		suffix := string(rune('a' + i))
		nodes := f.nodes("zone-a", sharedNode.Name, "h"+suffix)
		pods := f.workload("shop", "app"+suffix, 2, nodes...)
		f.fail(pods[0], detection.ModeCrashLoop, failingH, 2+i, "")
		if i == 0 {
			first = pods[0]
		}
	}
	return first
}

func TestWithheldSharedFactorIsNeverChosen(t *testing.T) {
	f := newFixture(t)
	first := threeOnSharedNode(f)
	requireCause(t, f.explain(), first, "node//n1")

	s := f.snapshot()
	s.NoSharedFactors = true
	got := Explain(s)

	for _, area := range got.Areas {
		for _, c := range append(area.Causes, area.Alternatives...) {
			if c.Root == sharedNode {
				t.Fatalf("withheld shared factor chosen: %+v", c)
			}
		}
	}
	for _, area := range got.Areas {
		if containsID(area.Withheld, first) {
			return
		}
	}
	t.Fatalf("the withheld shared factor must be recorded: %+v", got)
}

func TestSharedFactorIsNotWithheldByDefault(t *testing.T) {
	f := newFixture(t)
	threeOnSharedNode(f)
	for _, area := range f.explain().Areas {
		if len(area.Withheld) > 0 {
			t.Fatalf("nothing is withheld by default: %+v", area)
		}
	}
}

func TestOutrankedSharedFactorIsNotRecorded(t *testing.T) {
	f := newFixture(t)
	_, otherPod := configMapCase(f, 3)
	s := f.snapshot()
	s.NoSharedFactors = true
	for _, area := range Explain(s).Areas {
		if containsID(area.Withheld, otherPod) {
			t.Fatalf("a shared factor that is never chosen must not "+
				"be recorded: %+v", area)
		}
	}
}
