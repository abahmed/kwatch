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

const probeFailureMessage = "Readiness probe failed: HTTP probe failed " +
	"with statuscode: 500"

// neverReadyRig is a Deployment of the given replicas with one running,
// unready pod that became unready at t0 and has never restarted.
func neverReadyRig(
	replicas, ready float64,
) (*inventory.Model, inventory.EntityID, inventory.EntityID) {
	model := newTestModel()
	deploy := newID(kube.KindDeployment, "ops", "cert-controller")
	pod := newID(kube.KindPod, "ops", "cert-controller-1")
	put(model, deploy, t0, map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(replicas),
		kube.AttrReadyReplicas: inventory.Number(ready)})
	put(model, pod, t0, map[string]inventory.Value{
		kube.AttrPhase:      inventory.Text("Running"),
		kube.AttrReady:      inventory.Bool(false),
		kube.AttrReadySince: inventory.Time(t0)})
	relateEntity(model, pod, inventory.OwnedBy, deploy)
	return model, deploy, pod
}

func detectWorkloadAt(
	model *inventory.Model, id inventory.EntityID, after time.Duration,
) []detection.Finding {
	ctx := testDetectorContext(model, t0.Add(after))
	return NewWorkload(0).Detect(ctx, entityOf(model, id))
}

func neverReadyOf(found []detection.Finding) (detection.Finding, bool) {
	for _, f := range found {
		if f.Reason == reasons.WorkloadNeverReady {
			return f, true
		}
	}
	return detection.Finding{}, false
}

func TestWorkloadNeverReadyWarnsWhenNothingServes(t *testing.T) {
	model, deploy, pod := neverReadyRig(1, 0)
	warn(model, pod, t0.Add(time.Minute), "Unhealthy", probeFailureMessage)

	f, ok := neverReadyOf(detectWorkloadAt(model, deploy, 142*time.Minute))

	require.True(t, ok)
	assert.Equal(t, detection.Warning, f.Severity)
	assert.False(t, f.Symptom, "it is the cause, not a consequence")
	assert.Contains(t, f.Summary, "0 of 1 replicas are ready")
	assert.Contains(t, f.Summary, "2h22m")
	assert.Contains(t, f.Evidence,
		detection.Evidence{Label: "ready", Value: "0/1"})
	assert.Contains(t, f.Evidence, detection.Evidence{
		Label: "readiness probe", Value: probeFailureMessage})
}

func TestWorkloadNeverReadyWaitsForTheGrace(t *testing.T) {
	model, deploy, _ := neverReadyRig(1, 0)

	_, early := neverReadyOf(detectWorkloadAt(model, deploy,
		neverReadyGrace-time.Second))
	_, due := neverReadyOf(detectWorkloadAt(model, deploy, neverReadyGrace))

	assert.False(t, early)
	assert.True(t, due)
}

func TestWorkloadNeverReadyIsDigestNewsWhenSomeReplicasServe(t *testing.T) {
	model, deploy, _ := neverReadyRig(2, 1)

	f, ok := neverReadyOf(detectWorkloadAt(model, deploy, time.Hour))

	require.True(t, ok)
	assert.Equal(t, detection.Info, f.Severity)
	assert.NotContains(t, f.Evidence, detection.Evidence{
		Label: "readiness probe", Value: ""})
}

func TestWorkloadNeverReadyLeavesFailingPodsToTheirOwnFindings(t *testing.T) {
	tt := []struct {
		name  string
		phase string
		extra func(*inventory.Model, inventory.EntityID)
	}{
		{name: "pending_pod", phase: "Pending"},
		{name: "restarting_container", phase: "Running",
			extra: func(m *inventory.Model, pod inventory.EntityID) {
				c := newID(kube.KindContainer, "ops", "cert-controller-1/app")
				put(m, c, t0, map[string]inventory.Value{
					kube.AttrRestarts: inventory.Number(3)})
				relateEntity(m, c, inventory.PartOf, pod)
			}},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			model, deploy, pod := neverReadyRig(1, 0)
			put(model, pod, t0, map[string]inventory.Value{
				kube.AttrPhase: inventory.Text(tc.phase)})
			if tc.extra != nil {
				tc.extra(model, pod)
			}

			_, ok := neverReadyOf(detectWorkloadAt(model, deploy,
				time.Hour))

			assert.False(t, ok)
		})
	}
}

func TestWorkloadNeverReadyIgnoresReadyAndScaledToZero(t *testing.T) {
	model, deploy, _ := neverReadyRig(1, 1)
	_, ok := neverReadyOf(detectWorkloadAt(model, deploy, time.Hour))
	assert.False(t, ok, "all replicas ready")

	model, deploy, _ = neverReadyRig(0, 0)
	_, ok = neverReadyOf(detectWorkloadAt(model, deploy, time.Hour))
	assert.False(t, ok, "scaled to zero")
}
