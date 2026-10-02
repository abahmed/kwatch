package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestUsageSummaryIgnoresExactPercentage(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	summaries := map[string]bool{}
	for _, pct := range []float64{86, 91} {
		node := buildNode("n1", now, map[string]inventory.Value{
			kube.AttrFSUsedPct: inventory.Number(pct),
		})
		got := NodeUsage{}.Detect(testDetectorContext(nil, now), node)
		require.Len(t, got, 1)
		summaries[got[0].Summary] = true
		assert.Contains(t, evidenceValues(got[0]), "used")
	}
	assert.Len(t, summaries, 1, "the summary must not move with usage")
}

func TestCrashLoopSummaryIgnoresRestartCount(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	pod := inventory.EntityID{Kind: kube.KindPod, Namespace: "ns",
		Name: "p"}
	summaries := map[string]bool{}
	for _, restarts := range []float64{10, 11} {
		c := buildContainer(pod, "app", now, map[string]inventory.Value{
			kube.AttrState: inventory.Text("waiting"),
			kube.AttrStateReason: inventory.Text(
				reasons.CrashLoopBackOff),
			kube.AttrRestarts: inventory.Number(restarts),
		})
		got := Container{}.Detect(testDetectorContext(nil, now), c)
		require.Len(t, got, 1)
		summaries[got[0].Summary] = true
		assert.Contains(t, evidenceValues(got[0]), "restarts")
	}
	assert.Len(t, summaries, 1)
}

func evidenceValues(s detection.Finding) map[string]string {
	out := map[string]string{}
	for _, e := range s.Evidence {
		out[e.Label] = e.Value
	}
	return out
}
