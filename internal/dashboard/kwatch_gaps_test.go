package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/health"
)

func TestSpotLabelKnowsTheCloudsLabels(t *testing.T) {
	for label, want := range map[string]string{
		"karpenter.sh/capacity-type=spot":               "yes",
		"eks.amazonaws.com/capacityType=SPOT":           "yes",
		"cloud.google.com/gke-spot=true":                "yes",
		"cloud.google.com/gke-preemptible=true":         "yes",
		"kubernetes.azure.com/scalesetpriority=spot":    "yes",
		"node.kubernetes.io/lifecycle=spot":             "yes",
		"karpenter.sh/capacity-type=on-demand":          "",
		"eks.amazonaws.com/capacityType=ON_DEMAND":      "",
		"cloud.google.com/gke-spot=false":               "",
		"node.kubernetes.io/lifecycle=normal":           "",
		"kubernetes.io/os=linux":                        "",
		"kubernetes.azure.com/scalesetpriority=regular": "",
		"eks.amazonaws.com/capacityType=spot":           "yes",
	} {
		labels := map[string]string{}
		k, v, _ := strings.Cut(label, "=")
		labels[k] = v
		got := spotLabel(labels)
		assert.Equal(t, want != "", got != "", label)
		if want != "" {
			assert.Equal(t, k+"="+v, got)
		}
	}
	assert.Empty(t, spotLabel(nil))
}

func TestNotWatchingAddsTheGapsInsideTheWindow(t *testing.T) {
	now := testEpoch
	stops := []health.SelfStop{
		{At: now.Add(-time.Hour), StoppedAt: now.Add(-time.Hour - 4*time.Minute)},
		{At: now.Add(-3 * time.Hour), StoppedAt: now.Add(-3*time.Hour - time.Minute)},
		{At: now.Add(-5 * time.Hour)},
		{At: now.Add(-30 * time.Hour), StoppedAt: now.Add(-31 * time.Hour)},
		{At: now.Add(-23 * time.Hour), StoppedAt: now.Add(-25 * time.Hour)},
	}

	gap, unknown := notWatching(stops, now)

	assert.Equal(t, 5*time.Minute+time.Hour, gap,
		"4 min and 1 min, and the last gap cut to the window start")
	assert.Equal(t, 1, unknown, "a crash leaves no stop time")
}

func TestKwatchStatusServesTimeNotWatchingAndSpotNode(t *testing.T) {
	tm := newTestModel(t)
	tm.node("n1", "karpenter.sh/capacity-type=spot", "", nil)
	tm.node("n2", "karpenter.sh/capacity-type=on-demand", "", nil)
	s := newTestServer()
	s.SetModel(tm.m)
	s.SetSelf(func() (health.SelfReport, bool) {
		return health.SelfReport{Restarts: []health.SelfStop{{
			At:        testEpoch.Add(-time.Hour),
			StoppedAt: testEpoch.Add(-time.Hour - 3*time.Minute)}}}, true
	})

	got, body := readKwatch(t, s)
	assert.NotContains(t, body, "spotNode", "no node name, no notice")
	require.NotNil(t, got.NotWatchingSeconds)
	assert.Equal(t, 180, *got.NotWatchingSeconds)

	s.opts.NodeName = "n2"
	got, _ = readKwatch(t, s)
	assert.Nil(t, got.SpotNode, "an on-demand node gets no notice")

	s.opts.NodeName = "n1"
	got, _ = readKwatch(t, s)
	require.NotNil(t, got.SpotNode)
	assert.Equal(t, "karpenter.sh/capacity-type=spot", got.SpotNode.Label)
}
