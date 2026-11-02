package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

// An empty 200 answer is a blip like a failed poll: the readings stay
// for statsFailuresBeforeClear-1 answers and are cleared on the next.
func TestPublishLogGivesAnEmptyAnswerTheFailureGrace(t *testing.T) {
	log := newPublishLog()
	now := fixedNow()
	log.answered("a", []inventory.Observation{
		claimObservation(StatsSource)}, now)
	for i := 0; i < statsFailuresBeforeClear-1; i++ {
		assert.Empty(t, log.answered("a", nil, now))
	}
	assert.Len(t, log.answered("a", nil, now), 1)
	assert.Empty(t, log.owners)
}

func TestPublishLogKeepsAReadingAnotherNodeStillHolds(t *testing.T) {
	log := newPublishLog()
	now := fixedNow()
	claim := []inventory.Observation{claimObservation(StatsSource)}
	log.answered("a", claim, now)
	log.answered("b", claim, now) // b reported it last: b owns it
	// b stops reporting it, while a's last answer still holds it.
	var cleared []inventory.Observation
	for range statsFailuresBeforeClear {
		cleared = append(cleared, log.answered("b", nil, now)...)
	}
	assert.Empty(t, cleared, "a still reports the claim")
	key := published{claim[0].Entity, StatsSource}
	assert.Equal(t, "a", log.owners[key], "ownership moved to a")

	for range statsFailuresBeforeClear {
		cleared = append(cleared, log.answered("a", nil, now)...)
	}
	assert.Empty(t, log.owners)
	assert.Len(t, cleared, 1, "cleared once no node holds it")
}
