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

const preemptMessage = "batch/importer-x: preempting to accommodate a " +
	"higher priority pod"

// preemptFixture is a worker pod preempted two minutes ago by pod
// importer-x of the batch Job importer; the worker Deployment wants 3
// replicas and has ready.
func preemptFixture(
	t *testing.T, ready float64, message string,
) (*inventory.Model, inventory.EntityID) {
	t.Helper()
	model := newTestModel()
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "worker")
	observeEntity(model, deploy, podNodeNow.Add(-time.Hour),
		map[string]inventory.Value{
			kube.AttrReplicas:      inventory.Number(3),
			kube.AttrReadyReplicas: inventory.Number(ready),
		})
	job := inventory.CoreID(kube.KindJob, "batch", "importer")
	observeEntity(model, job, podNodeNow.Add(-time.Hour), nil)
	preemptor := inventory.CoreID(kube.KindPod, "batch", "importer-x")
	observeEntity(model, preemptor, podNodeNow.Add(-time.Hour),
		map[string]inventory.Value{
			kube.AttrPhase:    inventory.Text("Running"),
			kube.AttrPriority: inventory.Number(1000),
		})
	relateEntity(model, preemptor, inventory.OwnedBy, job)
	victim := inventory.CoreID(kube.KindPod, "shop", "worker-5f")
	attrs := map[string]inventory.Value{
		kube.AttrPhase:    inventory.Text("Failed"),
		kube.AttrPriority: inventory.Number(0),
	}
	conditionAttrs(attrs, "DisruptionTarget", "True",
		"PreemptionByScheduler", podNodeNow.Add(-2*time.Minute))
	if message != "" {
		attrs[kube.ConditionKey("DisruptionTarget")+
			kube.AttrConditionMessage] = inventory.Text(message)
	}
	observeEntity(model, victim, podNodeNow, attrs)
	relateEntity(model, victim, inventory.OwnedBy, deploy)
	return model, victim
}

func preemptedFinding(
	t *testing.T, model *inventory.Model, victim inventory.EntityID,
) []detection.Finding {
	t.Helper()
	return preemptedFindings(testDetectorContext(model, podNodeNow),
		entityOf(model, victim))
}

func preemptEvidence(f detection.Finding, label string) string {
	for _, e := range f.Evidence {
		if e.Label == label {
			return e.Value
		}
	}
	return ""
}

func TestPreemptedPodNamesThePreemptorAndPriorities(t *testing.T) {
	model, victim := preemptFixture(t, 2, preemptMessage)
	found := preemptedFinding(t, model, victim)

	require.Len(t, found, 1)
	assert.Equal(t, reasons.PodPreempted, found[0].Reason)
	assert.Equal(t, "batch/importer-x",
		preemptEvidence(found[0], detection.EvidencePreemptor))
	assert.Equal(t, "batch/importer",
		preemptEvidence(found[0], detection.EvidencePreemptorOwner))
	assert.Equal(t, "1000 > 0",
		preemptEvidence(found[0], detection.EvidencePriorities))
	assert.True(t, found[0].Since.Equal(podNodeNow.Add(-2*time.Minute)))
}

func TestPreemptedPodReadsTheSchedulersEvent(t *testing.T) {
	model, victim := preemptFixture(t, 2, "")
	noteEntity(model, victim, "Preempted",
		"Preempted by batch/importer-x on node n1", 1,
		podNodeNow.Add(-2*time.Minute))
	found := preemptedFinding(t, model, victim)

	require.Len(t, found, 1)
	assert.Equal(t, "batch/importer-x",
		preemptEvidence(found[0], detection.EvidencePreemptor))
}

func TestPreemptionWithoutALossIsNotReported(t *testing.T) {
	model, victim := preemptFixture(t, 3, preemptMessage)
	assert.Empty(t, preemptedFinding(t, model, victim),
		"every replica is back: capacity was not lost")
}

func TestPreemptionOlderThanTheWindowIsNotReported(t *testing.T) {
	model, victim := preemptFixture(t, 2, preemptMessage)
	later := podNodeNow.Add(time.Hour)
	assert.Empty(t, preemptedFindings(testDetectorContext(model, later),
		entityOf(model, victim)))
}

func TestPreemptedPodWithoutAPreemptorStillSaysItWasPreempted(t *testing.T) {
	model, victim := preemptFixture(t, 2, "")
	found := preemptedFinding(t, model, victim)

	require.Len(t, found, 1)
	assert.Empty(t, preemptEvidence(found[0], detection.EvidencePreemptor))
}
