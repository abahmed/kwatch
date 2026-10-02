package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestWorkloadReportsMissingPriorityClass(t *testing.T) {
	m := newTestModel()
	deploy := newID(kube.KindDeployment, "ns", "api")
	put(m, deploy, t0, nil)
	m.Apply(inventory.Observation{
		Kind: inventory.Related, Source: "test", At: t0, Entity: deploy,
		Relation: inventory.References, Targets: []inventory.EntityID{
			inventory.CoreID(kube.KindPriorityClass, "", "critical"),
			inventory.CoreID(kube.KindRuntimeClass, "", "gvisor"),
		},
	})
	put(m, inventory.CoreID(kube.KindRuntimeClass, "", "gvisor"), t0, nil)

	eval := evaluate(NewWorkload(0), m, t0, deploy, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, reasons.PriorityClassMissing, eval.Findings[0].Reason)
	assert.Equal(t, "Reference.PriorityClassMissing",
		string(eval.Findings[0].Mode))
	assert.Contains(t, eval.Findings[0].Summary, "critical")
}

func TestJobReportsMissingRuntimeClass(t *testing.T) {
	m := newTestModel()
	job := newID(kube.KindJob, "ns", "batch")
	put(m, job, t0, nil)
	link(m, job, inventory.References,
		inventory.CoreID(kube.KindRuntimeClass, "", "kata"))

	eval := evaluate(Job{}, m, t0, job, nil)

	require.Len(t, eval.Findings, 1)
	assert.Equal(t, "Reference.RuntimeClassMissing", string(eval.Findings[0].Mode))
}

func TestClassReferencesLeftToOwnerAndUnsyncedKinds(t *testing.T) {
	m := newTestModel()
	deploy := newID(kube.KindDeployment, "ns", "api")
	rs := newID(kube.KindReplicaSet, "ns", "api-1")
	put(m, deploy, t0, nil)
	put(m, rs, t0, nil)
	link(m, rs, inventory.OwnedBy, deploy)
	class := inventory.CoreID(kube.KindPriorityClass, "", "critical")
	link(m, rs, inventory.References, class)
	link(m, deploy, inventory.References, class)

	owned := evaluate(NewWorkload(0), m, t0, rs, nil)
	assert.Empty(t, owned.Findings, "the Deployment reports it")

	unsynced := func(kind inventory.Kind) bool {
		return kind != kube.KindPriorityClass
	}
	eval := evaluate(NewWorkload(0), m, t0, deploy, unsynced)
	assert.Empty(t, eval.Findings)
}

func TestMissingReportsPodRuntimeClassOnly(t *testing.T) {
	m := newTestModel()
	pod := newID(kube.KindPod, "ns", "sandboxed")
	put(m, pod, t0, nil)
	m.Apply(inventory.Observation{
		Kind: inventory.Related, Source: "test", At: t0, Entity: pod,
		Relation: inventory.References, Targets: []inventory.EntityID{
			inventory.CoreID(kube.KindPriorityClass, "", "gone"),
			inventory.CoreID(kube.KindRuntimeClass, "", "kata"),
		},
	})

	eval := evaluate(Missing{}, m, t0, pod, nil)

	require.Len(t, eval.Findings, 1, "running pods keep a deleted priority")
	assert.Equal(t, reasons.RuntimeClassMissing, eval.Findings[0].Reason)
}
