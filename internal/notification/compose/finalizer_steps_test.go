package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// A finalizer no controller in the cluster handles is never removed:
// each held object gets a read of its finalizers and a patch that drops
// only that finalizer, labelled as a change.
func TestOrphanFinalizerStepsDropOnlyTheOrphan(t *testing.T) {
	cause := unhandledCause(explain.ModeFinalizerUnhandled)
	cause.Score = 0.95
	f := heldFacts(cause, "shop", 1,
		"widgets.example.com/cleanup, kubernetes.io/pv-protection")

	steps := nextSteps(f.p, f.members)

	require.Len(t, steps, 2)
	assert.Equal(t, "kubectl get secret old-0 -n shop "+
		"-o jsonpath='{.metadata.finalizers}'", steps[0].Command)
	assert.False(t, steps[0].Mutating)
	assert.Equal(t, "kubectl patch secret old-0 -n shop --type=json "+
		`-p '[{"op":"test","path":"/metadata/finalizers/0",`+
		`"value":"widgets.example.com/cleanup"},`+
		`{"op":"remove","path":"/metadata/finalizers/0"}]'`,
		steps[1].Command)
	assert.Contains(t, steps[1].Text, "if the patch is refused, the "+
		"list changed, so read it again")
	assert.True(t, steps[1].Mutating)
	assert.Contains(t, steps[1].Text, "changes the cluster")
}

// The patch removes the orphan where kwatch saw it, after a test that
// it is still there: a list another controller changed since is
// refused, never rewritten from kwatch's copy.
func TestOrphanFinalizerPatchTestsItsPlace(t *testing.T) {
	cause := unhandledCause(explain.ModeFinalizerUnhandled)
	cause.Score = 0.95
	f := heldFacts(cause, "shop", 1, "kubernetes.io/pv-protection, "+
		"other.example.com/hold, widgets.example.com/cleanup")

	steps := nextSteps(f.p, f.members)

	require.Len(t, steps, 2)
	assert.Equal(t, "kubectl patch secret old-0 -n shop --type=json "+
		`-p '[{"op":"test","path":"/metadata/finalizers/2",`+
		`"value":"widgets.example.com/cleanup"},`+
		`{"op":"remove","path":"/metadata/finalizers/2"}]'`,
		steps[1].Command)
	assert.NotContains(t, steps[1].Command, "pv-protection")
}

// No patch while a controller known to handle the finalizer runs, while
// one is only scaled down, or for a cause below likely.
func TestOrphanFinalizerStepsNeedAGoneControllerAndASureCause(t *testing.T) {
	for name, f := range map[string]caseFacts{
		"handler runs": heldFacts(func() *rootcause.CauseRecord {
			c := unhandledCause(explain.ModeFinalizerHandlerRuns)
			c.Score = 0.95
			return c
		}(), "shop", 1, "widgets.example.com/cleanup"),
		"scaled down": heldFacts(func() *rootcause.CauseRecord {
			c := controllerCause(explain.ModeHandlerScaledDown)
			c.Score = 0.95
			return c
		}(), "shop", 1, "widgets.example.com/cleanup"),
		"possible": heldFacts(unhandledCause(
			explain.ModeFinalizerUnhandled), "shop", 1,
			"widgets.example.com/cleanup"),
	} {
		assert.Empty(t, orphanFinalizerSteps(f.p, f.members), name)
		for _, s := range nextSteps(f.p, f.members) {
			assert.NotContains(t, s.Command, "kubectl patch", name)
		}
	}
}
