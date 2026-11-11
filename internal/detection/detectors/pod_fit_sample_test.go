package detectors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func sampleIDs(n int) []inventory.EntityID {
	ids := make([]inventory.EntityID, n)
	for i := range ids {
		ids[i] = inventory.EntityID{Kind: kube.KindNode,
			Name: fmt.Sprintf("n%04d", i)}
	}
	return ids
}

// A cluster within the limit is checked whole; a larger one is sampled
// across its whole list, not from its alphabetic start.
func TestEvenSampleSpreadsOverTheList(t *testing.T) {
	small := sampleIDs(150)
	assert.Equal(t, small, evenSample(small, 200))

	big := sampleIDs(1000)
	got := evenSample(big, 200)
	assert.Len(t, got, 200)
	assert.Equal(t, big[0], got[0])
	assert.Equal(t, big[995], got[199])
	assert.Len(t, uniqueIDs(got), 200)
}

func uniqueIDs(ids []inventory.EntityID) map[inventory.EntityID]bool {
	out := map[inventory.EntityID]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out
}
