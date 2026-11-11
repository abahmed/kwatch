package detection

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

func flagged(advisory, symptom bool, place *Place) Finding {
	return Finding{Entity: inventory.EntityID{Kind: "Pod",
		Namespace: "ns", Name: "p"}, Reason: "PodFailed",
		Severity: Warning, Summary: "Pod failed",
		Advisory: advisory, Symptom: symptom, Place: place}
}

func observeTwice(first, second Finding) []Transition {
	tr := NewTracker()
	tr.Observe(first.Entity, []Finding{first})
	return tr.Observe(second.Entity, []Finding{second})
}

// A finding that stops being advisory is a failure now: it is a real
// change, so the incident that held it as a risk is told.
func TestAdvisoryFlipIsChanged(t *testing.T) {
	for _, flip := range []bool{true, false} {
		got := observeTwice(flagged(flip, false, nil),
			flagged(!flip, false, nil))

		require.Len(t, got, 1)
		assert.Equal(t, Changed, got[0].Kind)
		assert.Equal(t, !flip, got[0].Finding.Advisory)
	}
}

// A flipped symptom flag or a different setting only rewords: it is
// never announced but holders replace their copy.
func TestSymptomAndPlaceFlipsAreRefreshed(t *testing.T) {
	place := &Place{Path: "resources.limits.memory", Value: "1Gi"}
	other := &Place{Path: "resources.limits.memory", Value: "2Gi"}
	cases := map[string][2]Finding{
		"symptom":   {flagged(false, false, nil), flagged(false, true, nil)},
		"place set": {flagged(false, false, nil), flagged(false, false, place)},
		"place": {flagged(false, false, place),
			flagged(false, false, other)},
	}
	for name, pair := range cases {
		got := observeTwice(pair[0], pair[1])
		require.Len(t, got, 1, name)
		assert.Equal(t, Refreshed, got[0].Kind, name)
	}
	same := observeTwice(flagged(false, false, place),
		flagged(false, false, &Place{Path: place.Path, Value: place.Value}))
	assert.Empty(t, same, "an equal setting is no news")
}
