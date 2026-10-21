package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// sharedNodeCase fails one pod of each of three workloads on n1 with
// text; a replica of each runs healthy on another node.
func sharedNodeCase(f *fixture, text string) inventory.EntityID {
	var first inventory.EntityID
	for i := 0; i < 3; i++ {
		suffix := string(rune('a' + i))
		nodes := f.nodes("zone-a", sharedNode.Name, "h"+suffix)
		pods := f.workload("shop", "app"+suffix, 2, nodes...)
		f.fail(containerOf(pods[0]), detection.ModeCrashLoop, failingH,
			2+i, text)
		if i == 0 {
			first = containerOf(pods[0])
		}
	}
	return first
}

// TestSharedNodeIsNotSuspectedForProgramDefects: three workloads whose
// own code panics on one node share the node by chance; a node does not
// write a nil dereference into three programs.
func TestSharedNodeIsNotSuspectedForProgramDefects(t *testing.T) {
	f := newFixture(t)
	effect := sharedNodeCase(f, rcaPanic)
	if c, ok := f.explain().CauseOf(effect); ok &&
		c.Root.Kind == "node" {
		t.Fatalf("the node was suspected: %v at %.2f", c.Root,
			c.Confidence)
	}
}

// TestSharedNodeStaysSuspectedForSilentFailures: with no words in the
// errors the node shared by the failures is still the suspect.
func TestSharedNodeStaysSuspectedForSilentFailures(t *testing.T) {
	f := newFixture(t)
	effect := sharedNodeCase(f, "")
	requireCause(t, f.explain(), effect, "node//n1")
}
