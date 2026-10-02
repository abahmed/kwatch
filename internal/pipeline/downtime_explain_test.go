package pipeline

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A ConfigMap deleted while kwatch was down is still the cause of the
// pods that cannot start without it: the deletion is recorded on the
// absent object, dated at the snapshot, so it precedes the failure and
// the root-cause engine blames it.
func TestEngineDowntimeDeletionExplainsConfigError(t *testing.T) {
	start := time.Date(2026, 9, 29, 11, 0, 0, 0, time.UTC)
	// The last snapshot before a restart is minutes old, well inside
	// the causal window.
	snapshot := start.Add(-5 * time.Minute)
	before := inventory.NewModel(inventory.Options{})
	applyAll(t, before, kube.ConfigMapSchema{}, configMap("cfg"), snapshot)
	cfg := before.Entities(kube.KindConfigMap)[0]

	h := newHarness(t, start)
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("billing")
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{}, podMissingConfigMap("billing-a", rs.Name, start))
	h.engine.storage.saved = savedFingerprints(t, before, snapshot)
	h.engine.reconcileDowntime()

	h.run(start.Add(6*time.Minute), 10*time.Second)

	changes := h.engine.deps.Model.Changes(cfg, snapshot)
	if len(changes) != 1 || !changes[0].Deleted ||
		!changes[0].At.Equal(snapshot) {
		t.Fatalf("deletion not recorded: %+v", changes)
	}
	p := incidentOfRoot(t, h.engine.deps.Incidents, cfg)
	if p.Cause == nil || p.Cause.Root != cfg || p.Cause.Change == nil ||
		!p.Cause.Change.Deleted {
		t.Fatalf("cause = %+v, want the deletion of %s", p.Cause, cfg)
	}
}

// podMissingConfigMap is a pod that cannot start because the config map
// cfg it takes its environment from does not exist.
func podMissingConfigMap(name, owner string, since time.Time) *corev1.Pod {
	p := pod(name, owner, "n1", false, since)
	p.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
		ConfigMapRef: &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: "cfg"},
		},
	}}
	p.Status.ContainerStatuses[0].State = corev1.ContainerState{
		Waiting: &corev1.ContainerStateWaiting{
			Reason:  "CreateContainerConfigError",
			Message: `configmap "cfg" not found`,
		},
	}
	return p
}

// incidentOfRoot is the incident the manager keeps for root.
func incidentOfRoot(
	t *testing.T, m *incident.Manager, root inventory.EntityID,
) incident.Record {
	t.Helper()
	records := m.Export()
	for _, r := range records {
		if r.Root == root {
			return r
		}
	}
	t.Fatalf("no incident rooted at %s among %+v", root, records)
	return incident.Record{}
}
