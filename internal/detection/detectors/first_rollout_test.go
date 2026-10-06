package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// newDeployment is shop/api created at created, whose Available
// condition went False at availableSince, with one ReplicaSet per
// revision.
func newDeployment(
	created, availableSince time.Time, revisions int,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	api := newID(kube.KindDeployment, "shop", "api")
	put(m, api, created, map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(3),
		kube.AttrReadyReplicas: inventory.Number(0),
		kube.AttrCreated:       inventory.Time(created),
	})
	setCondition(m, api, "Available", "False", "MinimumReplicasUnavailable",
		"", availableSince)
	for i := 0; i < revisions; i++ {
		rs := newID(kube.KindReplicaSet, "shop", "api-"+string(rune('a'+i)))
		put(m, rs, created, nil)
		link(m, rs, inventory.OwnedBy, api)
	}
	return m, api
}

func unavailableFinding(m *inventory.Model, id inventory.EntityID,
) detection.Finding {
	got := NewWorkload(0).Detect(testDetectorContext(m, t0),
		entityOf(m, id))
	for _, f := range got {
		if f.Symptom {
			return f
		}
	}
	return detection.Finding{}
}

func TestWorkloadMarksAFirstRolloutThatNeverBecameHealthy(t *testing.T) {
	created := t0.Add(-12 * time.Minute)
	m, api := newDeployment(created, created.Add(time.Second), 1)

	f := unavailableFinding(m, api)

	assert.Contains(t, f.Evidence, detection.Evidence{
		Label: detection.EvidenceNeverHealthy,
		Value: created.UTC().Format(time.RFC3339)})
}

func TestWorkloadDoesNotMarkADeploymentWithEarlierRevisions(t *testing.T) {
	created := t0.Add(-12 * time.Minute)
	m, api := newDeployment(created, created.Add(time.Second), 2)

	f := unavailableFinding(m, api)

	assert.NotContains(t, evidenceByLabel(f.Evidence),
		detection.EvidenceNeverHealthy)
}

func TestWorkloadDoesNotMarkADeploymentThatWasAvailableOnce(t *testing.T) {
	created := t0.Add(-3 * time.Hour)
	m, api := newDeployment(created, t0.Add(-12*time.Minute), 1)

	f := unavailableFinding(m, api)

	require.NotEmpty(t, f.Reason, "the workload is still unavailable")
	assert.NotContains(t, evidenceByLabel(f.Evidence),
		detection.EvidenceNeverHealthy)
}
