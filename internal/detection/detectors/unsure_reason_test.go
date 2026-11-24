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

// withoutKinds is a sync function under which the kinds never sync, as
// when RBAC denies them or configuration turns their watch off.
func withoutKinds(kinds ...inventory.Kind) func(inventory.Kind) bool {
	return func(k inventory.Kind) bool {
		for _, off := range kinds {
			if k == off {
				return false
			}
		}
		return true
	}
}

func observeReplicas(m *inventory.Model, id inventory.EntityID,
	at time.Time, ready float64,
) {
	put(m, id, at, map[string]inventory.Value{
		kube.AttrReplicas:        inventory.Number(3),
		kube.AttrReadyReplicas:   inventory.Number(ready),
		kube.AttrUpdatedReplicas: inventory.Number(3),
	})
}

// Class kinds that never sync must not hold a workload's other
// findings: a recovered Deployment's finding clears, and a second
// outage waits out its own grace instead of reusing the first onset.
func TestUnsyncedClassKindsDoNotHoldWorkloadFindings(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindDeployment, "default", "app")
	r := detection.NewRegistry(withoutKinds(kube.KindPriorityClass,
		kube.KindRuntimeClass), NewWorkload(5*time.Minute))

	observeReplicas(m, id, t0, 1)
	r.Evaluate(m, t0, id)
	ev := r.Evaluate(m, t0.Add(10*time.Minute), id)
	require.Equal(t, []string{reasons.DeploymentUnavailable},
		reasonsOf(ev.Findings))
	previous := ev.Findings

	observeReplicas(m, id, t0.Add(11*time.Minute), 3)
	ev = r.Evaluate(m, t0.Add(12*time.Minute), id)
	assert.Empty(t, ev.Findings)
	assert.Empty(t, ev.UnsyncedBy, "no class is referenced")
	assert.Empty(t, ev.Keeping(previous), "the recovery must clear it")

	again := t0.Add(time.Hour)
	observeReplicas(m, id, again, 1)
	ev = r.Evaluate(m, again, id)
	assert.Empty(t, ev.Findings,
		"a new outage waits its grace, not the first outage's")
}

// A class that is referenced and absent while its kind is not synced
// leaves only the class finding unsure: the workload's other findings
// still clear.
func TestUnsyncedReferencedClassKeepsOnlyItsReason(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindDeployment, "default", "app")
	observeReplicas(m, id, t0, 3)
	link(m, id, inventory.References,
		inventory.CoreID(kube.KindPriorityClass, "", "high"))

	ev := evaluate(NewWorkload(0), m, t0, id,
		withoutKinds(kube.KindPriorityClass))

	assert.Empty(t, ev.Findings)
	assert.Empty(t, ev.UnsyncedBy)
	assert.Equal(t, []string{reasons.PriorityClassMissing},
		ev.UnsureReasons)
	assert.Equal(t, []inventory.Kind{kube.KindPriorityClass}, ev.Unsynced)
	kept := ev.Keeping([]detection.Finding{
		{Reason: reasons.PriorityClassMissing, Detector: "workload"},
		{Reason: reasons.DeploymentUnavailable, Detector: "workload"},
	})
	assert.Equal(t, []string{reasons.PriorityClassMissing},
		reasonsOf(kept))
}

// With Secrets unwatched (watch.secrets: false) an Ingress whose
// backend Service went missing and came back resolves.
func TestIngressBackendResolvesWithSecretsUnwatched(t *testing.T) {
	m := newTestModel()
	ing := newID(kube.KindIngress, "ns", "web")
	service := inventory.CoreID(kube.KindService, "ns", "checkout")
	put(m, ing, t0, nil)
	link(m, ing, inventory.RoutesTo, service)
	noSecrets := withoutKinds(kube.KindSecret)
	r := detection.NewRegistry(noSecrets, Ingress{})

	r.Evaluate(m, t0, ing)
	ev := r.Evaluate(m, t0.Add(DefaultBackendGrace), ing)
	require.Equal(t, []string{reasons.IngressBackendNotFound},
		reasonsOf(ev.Findings))
	previous := ev.Findings

	put(m, service, t0.Add(DefaultBackendGrace+time.Second), nil)
	ev = r.Evaluate(m, t0.Add(DefaultBackendGrace+2*time.Second), ing)
	assert.Empty(t, ev.Findings)
	assert.Empty(t, ev.UnsyncedBy)
	assert.Empty(t, ev.Keeping(previous))
}

// A ConfigMap a pod missed resolves once created, although the pod's
// Secrets are not watched.
func TestMissingConfigMapResolvesWithSecretsUnwatched(t *testing.T) {
	m := newTestModel()
	pod := newID(kube.KindPod, "ns", "web-0")
	config := inventory.CoreID(kube.KindConfigMap, "ns", "settings")
	put(m, pod, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Pending")})
	m.Apply(inventory.Observation{
		Kind: inventory.Related, Source: "test", At: t0, Entity: pod,
		Relation: inventory.References, Targets: []inventory.EntityID{
			config, inventory.CoreID(kube.KindSecret, "ns", "creds")},
	})
	r := detection.NewRegistry(withoutKinds(kube.KindSecret), Missing{})

	ev := r.Evaluate(m, t0, pod)
	require.Equal(t, []string{reasons.ProjectedConfigMapMissing},
		reasonsOf(ev.Findings))
	assert.Equal(t, []string{reasons.ProjectedSecretMissing},
		ev.UnsureReasons, "the unwatched Secret is unknown, not missing")
	previous := ev.Findings

	put(m, config, t0.Add(time.Minute), nil)
	ev = r.Evaluate(m, t0.Add(2*time.Minute), pod)
	assert.Empty(t, ev.Findings)
	assert.Empty(t, ev.Keeping(previous))
}

// A kind whose access was lost mid-run is not complete (the source's
// Complete): an object created after the loss is absent from the
// frozen cache, and must not be reported as not existing.
func TestAccessLostKindConcludesNothingMissing(t *testing.T) {
	m := newTestModel()
	pod := newID(kube.KindPod, "ns", "web-1")
	put(m, pod, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Pending")})
	link(m, pod, inventory.References,
		inventory.CoreID(kube.KindConfigMap, "ns", "created-after-loss"))
	lost := withoutKinds(kube.KindConfigMap)

	ev := evaluate(Missing{}, m, t0, pod, lost)
	assert.Empty(t, ev.Findings, "a frozen cache proves no absence")
	assert.Equal(t, []inventory.Kind{kube.KindConfigMap}, ev.Unsynced,
		"evaluated again once the kind is complete")
}
