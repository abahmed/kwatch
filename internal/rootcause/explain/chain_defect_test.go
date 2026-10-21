package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
)

// TestChainSkipsACallerWhoseErrorIsAProgramDefect: an api configured to
// call a failing database that crashes on a nil dereference is not
// failing because of the database; configuration alone does not make
// the call the cause of a defect in the program.
func TestChainSkipsACallerWhoseErrorIsAProgramDefect(t *testing.T) {
	f := newFixture(t)
	chainNode(f, 1)
	api := chainTier(f, "api", "postgres", false, f.nodes("zone-a", "n3")...)
	for _, pod := range api.pods {
		f.fail(containerOf(pod), detection.ModeCrashLoop, failingH, 4,
			rcaPanic)
	}

	e := f.explain()

	if c, ok := e.CauseOf(containerOf(api.pods[0])); ok &&
		c.Root.Name == "n1" {
		t.Fatalf("the database node was blamed for a program defect: "+
			"%v via %v", c.Root, hopNames(c.Hops))
	}
}

// TestChainKeepsACallerWithNoErrorOfItsOwn: the same caller that is only
// not ready, with no error of its own, still follows the database.
func TestChainKeepsACallerWithNoErrorOfItsOwn(t *testing.T) {
	f := newFixture(t)
	chainNode(f, 1)
	api := chainTier(f, "api", "postgres", false, f.nodes("zone-a", "n3")...)
	failTier(f, api, 3)

	requireCause(t, f.explain(), containerOf(api.pods[0]), "node//n1")
}
