package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestConflictingPodOfReadsThePodTheEventNames(t *testing.T) {
	note := inventory.Note{Reason: "ConflictingController",
		Message: "pod data/db-1 is not controlled by StatefulSet db"}
	id, ok := ConflictingPodOf(note)
	assert.True(t, ok)
	assert.Equal(t, inventory.CoreID(kube.KindPod, "data", "db-1"), id)

	note.Reason = "FailedCreate"
	_, ok = ConflictingPodOf(note)
	assert.False(t, ok, "only the conflict event names a pod")

	note.Reason, note.Message = "ConflictingController", "something else"
	_, ok = ConflictingPodOf(note)
	assert.False(t, ok)
}
