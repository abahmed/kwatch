package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A pressure condition that no workload feels waits for the digest.
func TestNodePressureThatAffectsNothingWaitsForTheDigest(t *testing.T) {
	r := newRig(t, Config{})
	node := inventory.CoreID(kube.KindNode, "", "n1")
	r.raise(at(0), sig(node, reasons.MemoryPressure, detection.Critical))

	r.tick(at(DefaultSettle))
	assert.Equal(t, Digest, r.only().Tier)
	assert.Equal(t, levelUnaffected, r.m.levelOf(r.m.incidents[r.only().ID]))
}

// A node that is lost is never held back by it.
func TestLostNodeIsNotUnaffected(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), lostNode("n1"))

	assert.False(t, unaffected(r.m.incidents[r.only().ID]))
}
