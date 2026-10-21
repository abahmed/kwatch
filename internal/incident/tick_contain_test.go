package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/metrics"
)

// One incident that panics on its tick costs that incident one tick. The
// decisions of the others, made in the same tick, still go out: before,
// the panic reached the pipeline's boundary and the whole tick was lost,
// every time, for as long as the bad incident lived.
func TestPanickingIncidentDoesNotCostTheOthersTheirDecisions(t *testing.T) {
	r := newRig(t, Config{Verifiable: func(kind inventory.Kind) bool {
		if kind == kube.KindNode {
			panic("cannot judge a node")
		}
		return true
	}})
	// The node incident is the older one and loses its only member, so
	// the tick asks whether its kind can be observed, and panics.
	r.raise(at(0), lostNode("n9"))
	r.clear(at(1), lostNode("n9"))
	r.raise(at(2), podSig("web"))
	panics := metrics.DefaultRegistry().PanicCount("incident")

	ds := r.tick(at(DefaultSettle + 2))

	wantAction(t, ds, Announce, ReasonSettled)
	assert.Equal(t, "web", ds[0].Incident.Root.Name)
	assert.Equal(t, panics+1,
		metrics.DefaultRegistry().PanicCount("incident"), "counted")
	require.NotPanics(t, func() { r.tick(at(DefaultSettle + 12)) },
		"the bad incident is skipped again, not fatal")
}
