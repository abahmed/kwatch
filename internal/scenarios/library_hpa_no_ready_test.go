package scenarios

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/replay"
)

// hpaNoReadyScenarios are autoscalers that cannot read metrics because
// the workload they scale has no ready pod, and the twin whose
// workload still serves.
func hpaNoReadyScenarios() []scenario {
	return []scenario{
		hpaNoReadyPodsJoinsWorkload(), hpaMetricsBesideServing(),
		hpaMetricsOutliveRecovery(),
	}
}

// unreadyPodsMetrics is the HPA controller's text when the pods it
// reads are not ready.
const unreadyPodsMetrics = "the HPA was unable to compute the replica " +
	"count: failed to get cpu utilization: did not receive metrics " +
	"for targeted pods (pods might be unready)"

// hpaFailing lists the shop web Deployment with its autoscaler, then
// makes count of its two pods crash-loop and the autoscaler fail to
// read its metrics for twenty minutes.
func hpaFailing(c *cluster, count int) {
	hpaFailingFor(c, count)
}

// hpaFailingFor is hpaFailing; it returns the Deployment so a story
// can go on.
func hpaFailingFor(c *cluster, count int) *workload {
	c.list(c.node("n1", "zone-a"))
	w := c.deployment("shop", "web", "registry.example.com/web:7", 2)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n1"))
	c.list(clusterHPA(c, "shop", "web", "True", "ValidMetricFound", ""))
	c.after(time.Minute)
	for i := range count {
		c.update(w.pod(i, "n1", crashLoop(1, "Error",
			"panic: web: cannot parse settings", 6)))
	}
	w.setReady(int32(2 - count))
	c.update(w.objects())
	hpa := clusterHPA(c, "shop", "web", "False",
		"FailedGetResourceMetric", unreadyPodsMetrics)
	c.update(hpa)
	for n := int32(1); n <= 20; n++ {
		c.warn(c.warningEvent(hpa, "HorizontalPodAutoscaler",
			"FailedGetResourceMetric", unreadyPodsMetrics,
			"horizontal-pod-autoscaler", n))
		c.after(time.Minute)
	}
	return w
}

// hpaMetricsOutliveRecovery: both pods of web crash-loop and its
// autoscaler cannot read metrics, as one incident. Then web is fixed
// and serves again, but the metrics still fail a quarter of an hour
// later. That is no longer web's failure: the incident moves to the
// autoscaler, which now fails on its own.
func hpaMetricsOutliveRecovery() scenario {
	return scenario{
		expect: expectation{
			Name: "hpa-metrics-outlive-recovery",
			Description: "Both pods of a Deployment crash-loop and its " +
				"HPA cannot read metrics; the pods recover and the " +
				"HPA still cannot read metrics 15 minutes later.",
			// The first message is about the crashing Deployment; the
			// story ends with the autoscaler alone.
			Root: "horizontalpodautoscaler/shop/web", RevisesRoot: true,
			Tier: "notify", MaxMessages: 3,
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			w := hpaFailingFor(c, 2)
			for i := range 2 {
				c.update(w.pod(i, "n1", startedNow))
			}
			w.setReady(2)
			c.update(w.objects())
			hpa := clusterHPA(c, "shop", "web", "False",
				"FailedGetResourceMetric", unreadyPodsMetrics)
			for n := int32(21); n <= 35; n++ {
				c.warn(c.warningEvent(hpa, "HorizontalPodAutoscaler",
					"FailedGetResourceMetric", unreadyPodsMetrics,
					"horizontal-pod-autoscaler", n))
				c.after(time.Minute)
			}
		},
	}
}

// hpaNoReadyPodsJoinsWorkload: both pods of web crash-loop, so its
// autoscaler has no metrics to read. That is the Deployment's failure
// seen from its autoscaler: one incident, not a second one for the HPA.
func hpaNoReadyPodsJoinsWorkload() scenario {
	return scenario{
		expect: expectation{
			Name: "hpa-no-ready-pods-joins-workload",
			Description: "Both pods of a Deployment crash-loop; its " +
				"HPA cannot read metrics from pods that are not ready.",
			Root: "deployment/shop/web", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"horizontalpodautoscaler/shop/web"},
			Tail:         duration(10 * time.Minute),
		},
		build: func(c *cluster) { hpaFailing(c, 2) },
	}
}

// hpaMetricsBesideServing: one of web's two pods crash-loops and the
// other still serves, while its autoscaler cannot read metrics. The
// workload does not explain the metrics: the autoscaler stays an
// incident of its own.
func hpaMetricsBesideServing() scenario {
	return scenario{
		expect: expectation{
			Name: "hpa-metrics-beside-serving",
			Description: "One of two pods of a Deployment crash-loops " +
				"and the other serves; its HPA cannot read metrics.",
			Root: "deployment/shop/web", Tier: "notify", MaxMessages: 3,
			OtherRoots: []string{"horizontalpodautoscaler/shop/web"},
			Tail:       duration(10 * time.Minute),
		},
		build: func(c *cluster) { hpaFailing(c, 1) },
	}
}

// hpaNoReadyRoots replays the HPA story with count crashing pods and
// lists the roots of the incidents it opened.
func hpaNoReadyRoots(t *testing.T, count int) []string {
	t.Helper()
	c := newCluster(scenarioStart, "")
	hpaFailing(c, count)
	result, err := replay.Run(context.Background(), c.log(),
		newDependencies(), replay.Options{SyncAt: scenarioStart})
	require.NoError(t, err)
	var roots []string
	for _, p := range result.Incidents {
		roots = append(roots, p.Root.String())
	}
	return roots
}

// The autoscaler of a workload with no ready pod opens no incident of
// its own; beside a workload that still serves, it does.
func TestHPAWithNoReadyPodsOpensNoIncidentOfItsOwn(t *testing.T) {
	down := hpaNoReadyRoots(t, 2)
	serving := hpaNoReadyRoots(t, 1)

	for _, root := range down {
		assert.NotContains(t, root, "horizontalpodautoscaler", down)
	}
	found := false
	for _, root := range serving {
		found = found || strings.Contains(root, "horizontalpodautoscaler")
	}
	assert.True(t, found, serving)
}
