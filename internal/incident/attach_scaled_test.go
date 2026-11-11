package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// scaledRig is a Deployment "shop" with ready replicas, a failing pod
// and an autoscaler that scales it.
func scaledRig(
	t *testing.T, ready float64,
) (r *rig, pod, deploy, hpa inventory.EntityID) {
	t.Helper()
	r = newRig(t, Config{})
	pod, deploy = workload(r, "shop")
	hpa = entity(kube.KindHPA, "shop")
	r.setAttrs(deploy, map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(2),
		kube.AttrReadyReplicas: inventory.Number(ready),
	})
	r.relate(hpa, inventory.Scales, deploy)
	return r, pod, deploy, hpa
}

func noMetrics(hpa inventory.EntityID) detection.Finding {
	return sig(hpa, reasons.FailedGetResourceMetric, detection.Warning)
}

// With no ready pod there are no metrics to read: the autoscaler's
// failure joins the workload's incident instead of opening its own.
func TestNoMetricsOfAWorkloadWithNoReadyPodJoinsItsIncident(t *testing.T) {
	r, pod, deploy, hpa := scaledRig(t, 0)
	r.raise(at(0), sig(pod, reasons.CrashLoopBackOff, detection.Critical))
	r.raise(at(0), noMetrics(hpa))

	got := r.only()
	assert.Equal(t, deploy, got.Root)
	assert.Contains(t, got.Members, noMetrics(hpa).Key())
}

// A workload that still has ready pods does not explain the metrics
// failure: the autoscaler keeps an incident of its own.
func TestNoMetricsOfAServingWorkloadStaysItsOwnIncident(t *testing.T) {
	r, pod, deploy, hpa := scaledRig(t, 1)
	r.raise(at(0), sig(pod, reasons.CrashLoopBackOff, detection.Critical))
	r.raise(at(0), noMetrics(hpa))

	assert.Equal(t, hpa, r.of(hpa).Root)
	assert.NotContains(t, r.of(deploy).Members, noMetrics(hpa).Key())
}

// Without an incident of the workload nothing changes.
func TestNoMetricsWithoutAWorkloadIncidentStaysItsOwn(t *testing.T) {
	r, _, _, hpa := scaledRig(t, 0)
	r.raise(at(0), noMetrics(hpa))

	assert.Equal(t, hpa, r.only().Root)
}

// joinedThenServing joins the autoscaler's failure to the workload's
// incident, then lets the workload recover at one minute while the
// metrics still fail.
func joinedThenServing(
	t *testing.T,
) (r *rig, deploy, hpa inventory.EntityID) {
	t.Helper()
	r, pod, deploy, hpa := scaledRig(t, 0)
	crash := sig(pod, reasons.CrashLoopBackOff, detection.Critical)
	r.raise(at(0), crash)
	r.raise(at(0), noMetrics(hpa))
	r.setAttrs(deploy, map[string]inventory.Value{
		kube.AttrReadyReplicas: inventory.Number(2)})
	r.clear(at(time.Minute), crash)
	return r, deploy, hpa
}

// The join does not last for good. When the workload has had ready
// pods for scaledSettle and the metrics still fail, the workload is
// not why: the autoscaler gets an incident of its own.
func TestNoMetricsLeavesAWorkloadThatServesAgain(t *testing.T) {
	r, deploy, hpa := joinedThenServing(t)

	tickEvery(r, time.Minute, time.Minute+scaledSettle-10*time.Second)
	assert.Equal(t, deploy, r.of(deploy).Root)
	assert.Contains(t, r.of(deploy).Members, noMetrics(hpa).Key(),
		"still joined while the metrics may come back")

	tickEvery(r, time.Minute+scaledSettle, 2*time.Minute+scaledSettle)

	own := r.of(hpa)
	assert.Equal(t, hpa, own.Root)
	assert.Contains(t, own.Members, noMetrics(hpa).Key())
}

// A workload that loses its pods again before scaledSettle keeps the
// autoscaler's failure: the wait starts over.
func TestNoMetricsStaysWhileTheWorkloadFlaps(t *testing.T) {
	r, deploy, hpa := joinedThenServing(t)
	tickEvery(r, time.Minute, 4*time.Minute)
	r.setAttrs(deploy, map[string]inventory.Value{
		kube.AttrReadyReplicas: inventory.Number(0)})
	r.tick(at(4*time.Minute + 10*time.Second))
	r.setAttrs(deploy, map[string]inventory.Value{
		kube.AttrReadyReplicas: inventory.Number(2)})

	tickEvery(r, 5*time.Minute, 8*time.Minute)

	assert.Contains(t, r.of(deploy).Members, noMetrics(hpa).Key())
}

// A release kept back while the workload's own root still fails is
// tried again later: once the workload is healthy no incident speaks
// for it any more and the autoscaler's failure stands on its own.
func TestNoMetricsLeavesOnceTheWorkloadRootRecovers(t *testing.T) {
	r, pod, deploy, hpa := scaledRig(t, 0)
	crash := sig(pod, reasons.CrashLoopBackOff, detection.Critical)
	down := sig(deploy, reasons.DeploymentUnavailable, detection.Critical)
	r.raise(at(0), crash, down)
	r.raise(at(0), noMetrics(hpa))
	tickEvery(r, 0, 6*time.Minute)
	r.setAttrs(deploy, map[string]inventory.Value{
		kube.AttrReadyReplicas: inventory.Number(1)})
	r.clear(at(6*time.Minute), crash)
	tickEvery(r, 6*time.Minute, 15*time.Minute)
	assert.Contains(t, r.of(deploy).Members, noMetrics(hpa).Key(),
		"kept while the deployment itself still fails")

	r.clear(at(16*time.Minute), down)
	tickEvery(r, 16*time.Minute, 20*time.Minute)

	assert.Equal(t, hpa, r.of(hpa).Root)
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	if p := r.m.lookup(deploy); p != nil {
		assert.Equal(t, Resolved, p.State, "a healthy workload")
	}
}
