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

// These tests keep one registry across evaluations, as the pipeline
// does, so the registry remembers when each bad state began.

func TestWorkloadWorseningOutageKeepsItsWait(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindDeployment, "shop", "web")
	replicas := func(at time.Time, ready float64) {
		put(m, id, at, map[string]inventory.Value{
			kube.AttrReplicas:      inventory.Number(3),
			kube.AttrReadyReplicas: inventory.Number(ready),
		})
	}
	registry := detection.NewRegistry(nil, NewWorkload(5*time.Minute))
	replicas(t0, 2)
	assert.Empty(t, registry.Evaluate(m, t0, id).Findings)
	// One more replica is lost four minutes into the outage.
	replicas(t0.Add(4*time.Minute), 1)
	assert.Empty(t,
		registry.Evaluate(m, t0.Add(4*time.Minute), id).Findings)

	got := registry.Evaluate(m, t0.Add(5*time.Minute), id).Findings
	require.Len(t, got, 1, "the wait runs from the first lost replica")
	assert.Equal(t, reasons.DeploymentUnavailable, got[0].Reason)
	assert.Equal(t, t0, got[0].Since)
}

func TestWorkloadRecoveryForgetsTheOutageStart(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindDeployment, "shop", "web")
	replicas := func(at time.Time, ready float64) {
		put(m, id, at, map[string]inventory.Value{
			kube.AttrReplicas:      inventory.Number(3),
			kube.AttrReadyReplicas: inventory.Number(ready),
		})
	}
	registry := detection.NewRegistry(nil, NewWorkload(5*time.Minute))
	replicas(t0, 2)
	registry.Evaluate(m, t0, id)
	replicas(t0.Add(time.Minute), 3)
	registry.Evaluate(m, t0.Add(time.Minute), id)
	// A new outage starts its own wait.
	replicas(t0.Add(3*time.Minute), 2)
	registry.Evaluate(m, t0.Add(3*time.Minute), id)
	assert.Empty(t,
		registry.Evaluate(m, t0.Add(6*time.Minute), id).Findings)
	got := registry.Evaluate(m, t0.Add(8*time.Minute), id).Findings
	require.Len(t, got, 1)
	assert.Equal(t, t0.Add(3*time.Minute), got[0].Since)
}

func TestServiceBackendLossKeepsItsWait(t *testing.T) {
	m, svc := serviceWithSlice(4, 3, "80", "80")
	slice := newID(kube.KindEndpointSlice, "default", "api-1")
	registry := detection.NewRegistry(nil, Service{})
	assert.Empty(t, registry.Evaluate(m, t0, svc).Findings)
	put(m, slice, t0.Add(3*time.Minute), map[string]inventory.Value{
		kube.AttrEndpoints:      inventory.Number(4),
		kube.AttrEndpointsReady: inventory.Number(2),
		kube.AttrEndpointPorts:  inventory.Text("80"),
	})
	assert.Empty(t,
		registry.Evaluate(m, t0.Add(3*time.Minute), svc).Findings)

	got := registry.Evaluate(m, t0.Add(5*time.Minute), svc).Findings
	require.Len(t, got, 1)
	assert.Equal(t, reasons.ServiceBackendsDegraded, got[0].Reason)
	assert.Equal(t, t0, got[0].Since)
}

func TestAPILatencyJitterKeepsItsWait(t *testing.T) {
	m := newTestModel()
	registry := detection.NewRegistry(nil, ClusterService{})
	// Every probe reports a slightly different slow latency.
	for i, latency := range []float64{2100, 2104, 2098, 2111} {
		at := t0.Add(time.Duration(i) * 30 * time.Second)
		put(m, kube.APIServer, at, map[string]inventory.Value{
			kube.AttrHealthy:   inventory.Bool(true),
			kube.AttrLatencyMS: inventory.Number(latency),
		})
		got := registry.Evaluate(m, at, kube.APIServer).Findings
		if i < 3 {
			assert.Empty(t, got, "probe %d", i)
			continue
		}
		require.Len(t, got, 1, "90s over the threshold since the first")
		assert.Equal(t, reasons.APIServerLatency, got[0].Reason)
		assert.Equal(t, t0, got[0].Since)
	}
}

// A condition without a known start waits its full grace from when it
// was first seen, instead of firing at once.
func TestSustainedWithoutStartWaitsFromFirstSighting(t *testing.T) {
	registry := detection.NewRegistry(nil, waitDetector{})
	m := newTestModel()
	id := newID("widget", "default", "thing")
	put(m, id, t0, nil)
	first := registry.Evaluate(m, t0, id)
	assert.Empty(t, first.Findings)
	assert.Equal(t, time.Minute, first.RecheckAfter)
	assert.Empty(t, registry.Evaluate(m, t0.Add(30*time.Second),
		id).Findings)
	assert.Len(t, registry.Evaluate(m, t0.Add(time.Minute), id).Findings, 1)
}

// waitDetector reports a condition of unknown start once it held for a
// minute.
type waitDetector struct{}

func (waitDetector) Name() string { return "wait" }
func (waitDetector) Kinds() []inventory.Kind {
	return []inventory.Kind{"widget"}
}
func (waitDetector) Detect(
	ctx detection.Context, _ inventory.Entity,
) []detection.Finding {
	if !sustained(ctx, "wait", time.Time{}, time.Minute) {
		return nil
	}
	return []detection.Finding{{Reason: "Waited",
		Severity: detection.Warning}}
}

// CPU usage and throttling change with every sample; the ten-minute wait
// runs from when each first went over its level, not from the latest
// sample.
func TestContainerCPUJitterKeepsItsWait(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindContainer, "default", "web/app")
	registry := detection.NewRegistry(nil, ContainerResources{})
	// The limit is old; only usage and throttling went bad at t0.
	put(m, id, t0.Add(-time.Hour), map[string]inventory.Value{
		kube.AttrCPUUsageMilli: inventory.Number(10),
		kube.AttrCPULimit:      inventory.Number(100),
		kube.AttrThrottledPct:  inventory.Number(0),
	})
	var got []detection.Finding
	for minute := 0; minute <= 10; minute++ {
		at := t0.Add(time.Duration(minute) * time.Minute)
		put(m, id, at, map[string]inventory.Value{
			kube.AttrCPUUsageMilli: inventory.Number(float64(95 + minute%2)),
			kube.AttrCPULimit:      inventory.Number(100),
			kube.AttrThrottledPct:  inventory.Number(float64(60 + minute%2)),
		})
		got = registry.Evaluate(m, at, id).Findings
		if minute < 10 {
			assert.Empty(t, got, "minute %d", minute)
		}
	}
	require.Len(t, got, 2, "both states lasted ten minutes")
	for _, finding := range got {
		assert.Equal(t, t0, finding.Since, finding.Reason)
	}
}

// Two bad states of unknown start keep their own first sighting: the
// second one does not inherit the first one's wait.
func TestSustainedKeysAreDistinct(t *testing.T) {
	registry := detection.NewRegistry(nil, twoWaitDetector{})
	m := newTestModel()
	id := newID("widget", "default", "thing")
	put(m, id, t0, map[string]inventory.Value{"a": inventory.Bool(true)})
	registry.Evaluate(m, t0, id)
	at := t0.Add(50 * time.Second)
	put(m, id, at, map[string]inventory.Value{
		"a": inventory.Bool(true), "b": inventory.Bool(true),
	})
	registry.Evaluate(m, at, id)
	got := registry.Evaluate(m, t0.Add(time.Minute), id).Findings
	require.Len(t, got, 1, "only a waited a minute")
	assert.Equal(t, "A", got[0].Reason)
}

type twoWaitDetector struct{}

func (twoWaitDetector) Name() string { return "two-wait" }
func (twoWaitDetector) Kinds() []inventory.Kind {
	return []inventory.Kind{"widget"}
}
func (twoWaitDetector) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	for key, reason := range map[string]string{"a": "A", "b": "B"} {
		if flag(e, key) &&
			sustained(ctx, key, time.Time{}, time.Minute) {
			out = append(out, detection.Finding{Reason: reason,
				Severity: detection.Warning})
		}
	}
	return out
}
