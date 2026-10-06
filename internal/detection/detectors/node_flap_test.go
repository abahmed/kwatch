package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// flapNode is a node whose Ready condition the test flips.
type flapNode struct {
	t        *testing.T
	model    *inventory.Model
	registry *detection.Registry
	id       inventory.EntityID
	start    time.Time
}

func newFlapNode(t *testing.T) *flapNode {
	return &flapNode{
		t: t, model: newTestModel(),
		registry: detection.NewRegistry(nil, NewNode(90*time.Second)),
		id:       inventory.EntityID{Kind: kube.KindNode, Name: "n1"},
		start:    time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}
}

// at sets the node's Ready status as of offset and evaluates it then.
func (n *flapNode) at(offset time.Duration, status string) []detection.Finding {
	now := n.start.Add(offset)
	setCondition(n.model, n.id, "Ready", status, "Kubelet", "msg", now)
	return n.registry.Evaluate(n.model, now, n.id).Findings
}

// recheck evaluates again without changing the condition.
func (n *flapNode) recheck(offset time.Duration) []detection.Finding {
	return n.registry.Evaluate(n.model, n.start.Add(offset), n.id).Findings
}

func TestNodeBlipShorterThanThresholdIsNoFinding(t *testing.T) {
	n := newFlapNode(t)
	assert.Empty(t, n.at(0, "True"))
	assert.Empty(t, n.at(time.Minute, "False"))
	assert.Empty(t, n.recheck(time.Minute+80*time.Second))
	assert.Empty(t, n.at(time.Minute+85*time.Second, "True"))
}

func TestNodeShortFlapsNeverAddUp(t *testing.T) {
	n := newFlapNode(t)
	assert.Empty(t, n.at(0, "True"))
	assert.Empty(t, n.at(10*time.Second, "False"))
	assert.Empty(t, n.at(60*time.Second, "True"))
	assert.Empty(t, n.at(80*time.Second, "False"))
	assert.Empty(t, n.at(130*time.Second, "True"))
	assert.Empty(t, n.at(150*time.Second, "False"))
	assert.Empty(t, n.recheck(200*time.Second))
}

func TestNodeFlapAfterReportedResumesAtOnce(t *testing.T) {
	n := newFlapNode(t)
	n.at(0, "False")
	assert.Len(t, n.recheck(2*time.Minute), 1)
	assert.Empty(t, n.at(3*time.Minute, "True"))
	found := n.at(3*time.Minute+30*time.Second, "False")
	if assert.Len(t, found, 1) {
		assert.True(t, found[0].Since.Equal(n.start))
	}
}

func TestNodeLongReadyForgetsTheEpisode(t *testing.T) {
	n := newFlapNode(t)
	n.at(0, "False")
	assert.Len(t, n.recheck(2*time.Minute), 1)
	assert.Empty(t, n.at(3*time.Minute, "True"))
	assert.Empty(t, n.recheck(3*time.Minute+90*time.Second))
	assert.Empty(t, n.recheck(3*time.Minute+150*time.Second))
	// Ready for 3 minutes: the next NotReady is a new episode.
	assert.Empty(t, n.at(7*time.Minute, "False"))
}
