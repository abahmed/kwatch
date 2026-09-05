package correlation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestFreezeAndSnapshotPersistedRejectsLateMutations(t *testing.T) {
	e := newTestEngine()
	ev := event.Event{
		Namespace: "default",
		PodName:   "pod-1",
		Reason:    "CrashLoopBackOff",
	}
	inc, action := e.Process(ev, "deployment-1", nil)
	assert.Equal(t, model.ActionCreate, action)

	snapshot := e.FreezeAndSnapshotPersisted()
	assert.Len(t, snapshot, 1)
	assert.Equal(t, 1, snapshot[0].Count)

	late, lateAction := e.Process(ev, "deployment-1", nil)
	e.MarkResolved(inc.Key)

	assert.Nil(t, late)
	assert.Equal(t, model.ActionSkip, lateAction)
	assert.Equal(t, snapshot, e.SnapshotPersisted())
}
