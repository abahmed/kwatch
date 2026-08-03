package context

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewChangeTracker(t *testing.T) {
	tr := NewChangeTracker(0)
	assert.NotNil(t, tr)
	assert.Equal(t, defaultTrackedChanges, len(tr.buffer))

	tr2 := NewChangeTracker(50)
	assert.Equal(t, 50, len(tr2.buffer))
}

func TestRecentChangesBefore(t *testing.T) {
	tr := NewChangeTracker(100)
	now := time.Now()
	tr.Record(Change{Resource: "pod", Name: "p1", Timestamp: now})
	tr.Record(Change{Resource: "pod", Name: "p2", Timestamp: now.Add(-10 * time.Minute)})
	tr.Record(Change{Resource: "pod", Name: "p3", Timestamp: now.Add(-20 * time.Minute)})

	recent := tr.RecentChangesBefore(15 * time.Minute)
	assert.Len(t, recent, 2)
}

func TestChangeTypeString(t *testing.T) {
	assert.Equal(t, "created", ChangeCreate.String())
	assert.Equal(t, "updated", ChangeUpdate.String())
	assert.Equal(t, "deleted", ChangeDelete.String())

	var unknown ChangeType = 99
	assert.Equal(t, "unknown", unknown.String())
}
