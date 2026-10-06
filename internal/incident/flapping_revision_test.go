package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A flapping incident says nothing about a revised cause, so the owed
// update must not wait for the incident to resolve and come back: the
// resolve drops it.
func TestFlappingResolveClearsTheOwedRevision(t *testing.T) {
	r := newRig(t, Config{})
	runCycles(r, 2*time.Minute, 4*time.Minute, 19*time.Minute)
	require.Equal(t, Flapping, r.only().State)

	web := podSig("web")
	node := entity(kube.KindNode, "n1")
	r.cause(web.Entity, node, "node n1 is out of memory")
	r.apply(at(19*time.Minute+10*time.Second), detection.Changed, web)
	r.tick(at(19*time.Minute + 15*time.Second))
	require.True(t, r.m.Export()[0].Revised, "the revision is owed")

	r.clear(at(20*time.Minute), web)
	var resolved bool
	for now := 20 * time.Minute; now < 2*time.Hour; now += 10 * time.Second {
		for _, d := range r.tick(at(now)) {
			resolved = resolved || d.Action == Resolve
		}
	}
	require.True(t, resolved)
	assert.False(t, r.m.Export()[0].Revised,
		"a resolved incident owes nothing")
}
