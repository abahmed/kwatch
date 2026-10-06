package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// A regression names what its revision changed, ranked, as evidence the
// writer reads: the template edits made before the new pods started.
func TestReleaseWatchListsWhatTheRevisionChanged(t *testing.T) {
	m, dep := buildRelease(t, releaseCase{
		restarts: [2]float64{2, 2}, age: 10 * time.Minute,
	})
	edit := func(path, before, after string) inventory.FieldChange {
		return inventory.FieldChange{Path: path, Before: before, After: after}
	}
	_, err := m.Apply(inventory.Observation{Kind: inventory.Changed,
		Source: "test", At: t0.Add(-11 * time.Minute), Entity: dep,
		Change: inventory.Change{Entity: dep, At: t0.Add(-11 * time.Minute),
			Fields: []inventory.FieldChange{
				edit("containers[app].args", "", "--fast"),
				edit("containers[app].image", "app:1", "app:2"),
				edit("spec.replicas", "2", "3")}}})
	require.NoError(t, err)

	got := evaluate(ReleaseWatch{}, m, t0, dep, nil).Findings

	require.Len(t, got, 1)
	labels := map[string]string{}
	for _, e := range got[0].Evidence {
		labels[e.Label] = e.Value
	}
	assert.Equal(t, "app:1"+detection.EvidenceEditArrow+"app:2",
		labels[detection.EvidenceEditPrefix+"containers[app].image"])
	assert.Equal(t, "unset"+detection.EvidenceEditArrow+"--fast",
		labels[detection.EvidenceEditPrefix+"containers[app].args"])
	assert.NotContains(t, labels,
		detection.EvidenceEditPrefix+"spec.replicas")
	var order []string
	for _, e := range got[0].Evidence {
		order = append(order, e.Label)
	}
	assert.Less(t, indexOf(order,
		detection.EvidenceEditPrefix+"containers[app].image"),
		indexOf(order, detection.EvidenceEditPrefix+"containers[app].args"),
		"the image is the likelier culprit")
}

func indexOf(values []string, want string) int {
	for i, v := range values {
		if v == want {
			return i
		}
	}
	return -1
}
