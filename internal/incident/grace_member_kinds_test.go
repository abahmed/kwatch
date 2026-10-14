package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A restored workload incident has no members until its pods are listed.
// A synced Deployment with an unsynced Pod list says nothing about
// recovery, so the incident stays open.
func TestRestoredWorkloadWaitsForItsPodKind(t *testing.T) {
	root := entity(kube.KindDeployment, "web")
	old := newRig(t, Config{})
	announced(t, old, sig(root, "Unavailable", detection.Warning))

	sight := &kindSight{hidden: map[inventory.Kind]bool{kube.KindPod: true}}
	fresh := newRig(t, Config{Verifiable: sight.verifiable})
	fresh.m.Restore(old.m.Export(), at(10*time.Minute))

	wantNone(t, fresh.tick(at(10*time.Minute)))
	wantNone(t, fresh.tick(at(30*time.Minute)))
	assert.Equal(t, Open, fresh.only().State,
		"pods are not listed yet: no proof of recovery")

	sight.hidden = nil
	wantNone(t, fresh.tick(at(31*time.Minute)))
	assert.Equal(t, Recovering, fresh.only().State)
}

// The grace can be moved later once the sources synced, never earlier.
func TestExtendGraceOnlyMovesLater(t *testing.T) {
	old := newRig(t, Config{})
	announced(t, old, podSig("web"))
	fresh := newRig(t, Config{})
	fresh.m.Restore(old.m.Export(), at(10*time.Minute))

	fresh.m.ExtendGrace(at(25 * time.Minute))
	wantNone(t, fresh.tick(at(12*time.Minute)))
	assert.Equal(t, Open, fresh.only().State)

	fresh.m.ExtendGrace(at(5 * time.Minute))
	wantNone(t, fresh.tick(at(13*time.Minute)))
	assert.Equal(t, Open, fresh.only().State, "never moved earlier")
}

// Without a restored live incident there is nothing to wait for.
func TestExtendGraceWithoutRestoredIncidents(t *testing.T) {
	r := newRig(t, Config{})
	r.m.ExtendGrace(at(25 * time.Minute))
	assert.True(t, r.m.restoreGrace.IsZero())
}
