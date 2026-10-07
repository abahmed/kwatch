package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const (
	pvcLeft = "Some content in the namespace has finalizers remaining: " +
		"kubernetes.io/pvc-protection in 3 resource instances"
	contentLeft = "Some resources are remaining: " +
		"persistentvolumeclaims. has 3 resource instances"
)

func terminatingNamespace(m *inventory.Model) inventory.EntityID {
	id := newID(kube.KindNamespace, "", "old-shop")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrPhase:      inventory.Text("Terminating"),
		kube.AttrFinalizers: inventory.Text("kubernetes"),
	})
	for _, c := range []struct{ kind, message string }{
		{"NamespaceContentRemaining", contentLeft},
		{"NamespaceFinalizersRemaining", pvcLeft},
	} {
		setCondition(m, id, c.kind, "True", "SomeResourcesRemain", "", t0)
		entity, _ := m.Entity(id)
		attrs := map[string]inventory.Value{}
		for k, a := range entity.Attributes {
			attrs[k] = a.Value
		}
		attrs[kube.ConditionKey(c.kind)+kube.AttrConditionMessage] =
			inventory.Text(c.message)
		put(m, id, t0, attrs)
	}
	return id
}

func TestStuckNamespaceQuotesWhatIsLeft(t *testing.T) {
	m := newTestModel()
	id := terminatingNamespace(m)

	got := evaluate(Namespace{}, m, t0.Add(2*time.Hour), id, nil).Findings

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "terminating for 2h")
	assert.Contains(t, got[0].Summary, pvcLeft)
	assert.Contains(t, got[0].Summary, contentLeft)
	values := evidenceValues(got[0])
	assert.Equal(t, "kubernetes", values["finalizers"])
	assert.Equal(t, pvcLeft, values["NamespaceFinalizersRemaining"])
}

func TestStuckNamespaceWithoutConditionsStillReports(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNamespace, "", "gone")
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Terminating"),
	})

	got := evaluate(Namespace{}, m, t0.Add(time.Hour), id, nil).Findings

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "terminating for 1h")
}

// The deletion request, not the moment kwatch saw the phase, starts the
// clock: a namespace deleted yesterday is not "terminating for a minute".
func TestStuckNamespaceCountsFromTheDeletionRequest(t *testing.T) {
	m := newTestModel()
	id := terminatingNamespace(m)
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrPhase:         inventory.Text("Terminating"),
		kube.AttrDeletingSince: inventory.Time(t0.Add(-5 * time.Hour)),
	})

	got := evaluate(Namespace{}, m, t0.Add(time.Minute), id, nil).Findings

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "terminating for 5h")
}
