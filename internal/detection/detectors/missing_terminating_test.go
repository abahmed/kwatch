package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// missingFindings evaluates the missing-reference detector at at.
func missingFindings(
	m *inventory.Model, at time.Time, id inventory.EntityID,
) []detection.Finding {
	r := detection.NewRegistry(nil, Missing{})
	return r.Evaluate(m, at, id).Findings
}

func TestMissingSkipsPodsOfATerminatingNamespace(t *testing.T) {
	m, pod := podMissingSecret(nil)
	put(m, newID(kube.KindNamespace, "", "default"), t0,
		map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Terminating")})

	assert.Empty(t, missingFindings(m, t0, pod.ID),
		"namespace deletion removes Secrets before pods")
}

// The control plane publishes kube-root-ca.crt and the default
// ServiceAccount a moment after a namespace. A pod that references
// them before then is early, not broken; once the moment is over a
// recheck reports what is still missing.
func TestMissingWaitsForBootstrapObjectsOfANewNamespace(t *testing.T) {
	m := newTestModel()
	pod := newID(kube.KindPod, "fresh", "pod")
	put(m, newID(kube.KindNamespace, "", "fresh"), t0,
		map[string]inventory.Value{kube.AttrCreated: inventory.Time(t0)})
	put(m, pod, t0, nil)
	m.Apply(inventory.Observation{
		Kind: inventory.Related, Source: "test", At: t0, Entity: pod,
		Relation: inventory.References, Targets: []inventory.EntityID{
			newID(kube.KindConfigMap, "fresh", kube.RootCAConfigMap),
			newID(kube.KindAccount, "fresh", kube.DefaultServiceAccount)},
	})
	r := detection.NewRegistry(nil, Missing{})

	early := r.Evaluate(m, t0.Add(10*time.Second), pod)
	assert.Empty(t, early.Findings, "published a moment later")
	assert.Equal(t, kube.BootstrapPublish-10*time.Second,
		early.RecheckAfter, "looked at again once the moment is over")

	late := Missing{}.Detect(testDetectorContext(m,
		t0.Add(kube.BootstrapPublish)), entityOf(m, pod))
	assert.Len(t, late, 2, "still missing well after: real findings")
}
