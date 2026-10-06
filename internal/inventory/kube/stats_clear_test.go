package kube

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

// summaryKubelet serves a changeable summary for node "a".
type summaryKubelet struct {
	summary *string
	err     *error
}

func (k summaryKubelet) Open(
	_ context.Context, _, path string,
) (io.ReadCloser, error) {
	if *k.err != nil {
		return nil, *k.err
	}
	if path != "stats/summary" {
		return nil, errors.New("not served")
	}
	return io.NopCloser(strings.NewReader(*k.summary)), nil
}

const summaryWithVolume = `{"node":{"fs":{"capacityBytes":100,
"usedBytes":90}},"pods":[{"podRef":{"name":"p","namespace":"ns"},
"volume":[{"usedBytes":95,"capacityBytes":100,
"pvcRef":{"name":"data","namespace":"ns"}}]}]}`

const summaryWithoutPod = `{"node":{"fs":{"capacityBytes":100,
"usedBytes":90}},"pods":[]}`

// clearHarness feeds the poller's observations into a model, the way the
// pipeline does.
type clearHarness struct {
	t       *testing.T
	poller  *StatsPoller
	model   *inventory.Model
	summary string
	err     error
	now     time.Time
}

func newClearHarness(t *testing.T) *clearHarness {
	h := &clearHarness{t: t, now: fixedNow(), summary: summaryWithVolume}
	h.model = inventory.NewModel(inventory.Options{
		EnrichmentSources: EnrichmentSources(),
	})
	h.poller = NewStatsPoller(StatsConfig{
		Kubelet: summaryKubelet{summary: &h.summary, err: &h.err},
		Now:     func() time.Time { return h.now },
		Nodes:   statsNodes("a"),
		Submit: func(_ context.Context, obs ...inventory.Observation) {
			for _, o := range obs {
				_, err := h.model.Apply(o)
				require.NoError(t, err)
			}
		},
	})
	for _, id := range []inventory.EntityID{
		inventory.CoreID(KindNode, "", "a"),
		inventory.CoreID(KindPVC, "ns", "data"),
	} {
		_, err := h.model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: ObservationSource,
			At: h.now, Entity: id,
		})
		require.NoError(t, err)
	}
	return h
}

func (h *clearHarness) pollAfter(d time.Duration) {
	h.now = h.now.Add(d)
	h.poller.poll(context.Background())
}

func (h *clearHarness) volumePct() bool {
	entity, ok := h.model.Entity(inventory.CoreID(KindPVC, "ns", "data"))
	require.True(h.t, ok)
	_, has := entity.Attributes[AttrVolumeUsedPct]
	return has
}

func TestStatsClearsVolumeWhenItLeavesTheNodeSummary(t *testing.T) {
	h := newClearHarness(t)
	h.pollAfter(time.Minute)
	require.True(t, h.volumePct())

	h.summary = summaryWithoutPod
	h.pollAfter(time.Minute)
	assert.False(t, h.volumePct(), "a PVC whose pod is gone has no usage")
}

func TestStatsClearsAttributesAfterRepeatedFailures(t *testing.T) {
	h := newClearHarness(t)
	h.pollAfter(time.Minute)
	require.True(t, h.volumePct())

	h.err = errors.New("dial tcp: i/o timeout")
	for i := 0; i < statsFailuresBeforeClear-1; i++ {
		h.pollAfter(time.Minute)
		assert.True(t, h.volumePct(), "one blip keeps the last reading")
	}
	h.pollAfter(time.Minute)
	assert.False(t, h.volumePct(), "an unreachable kubelet reads nothing")
	node, _ := h.model.Entity(inventory.CoreID(KindNode, "", "a"))
	_, has := node.Attributes[AttrFSUsedPct]
	assert.False(t, has)
}
