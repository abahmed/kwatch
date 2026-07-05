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

func TestRecordAndRecent(t *testing.T) {
	tr := NewChangeTracker(10)
	tr.Record(Change{Resource: "pod", Namespace: "ns1", Name: "p1", Type: ChangeCreate, Timestamp: time.Now()})
	tr.Record(Change{Resource: "node", Name: "n1", Type: ChangeUpdate, Timestamp: time.Now()})
	tr.Record(Change{Resource: "pod", Namespace: "ns1", Name: "p2", Type: ChangeDelete, Timestamp: time.Now()})

	recent := tr.Recent(0)
	assert.Len(t, recent, 3)
	assert.Equal(t, "p2", recent[2].Name)

	recent2 := tr.Recent(2)
	assert.Len(t, recent2, 2)
}

func TestRecentEmptyTracker(t *testing.T) {
	tr := NewChangeTracker(10)
	assert.Empty(t, tr.Recent(5))
	assert.Empty(t, tr.RecentByResource("pod", 5))
	assert.Empty(t, tr.RecentChangesBefore(time.Minute))
}

func TestRingBufferWrapping(t *testing.T) {
	tr := NewChangeTracker(3)
	for i := 0; i < 5; i++ {
		tr.Record(Change{Resource: "pod", Name: "p"})
	}
	assert.Equal(t, 3, tr.count)
	assert.Equal(t, 3, len(tr.Recent(10)))
}

func TestRecentByResource(t *testing.T) {
	tr := NewChangeTracker(100)
	tr.Record(Change{Resource: "pod", Namespace: "ns1", Name: "p1"})
	tr.Record(Change{Resource: "node", Namespace: "", Name: "n1"})
	tr.Record(Change{Resource: "pod", Namespace: "ns1", Name: "p2"})
	tr.Record(Change{Resource: "configmap", Namespace: "ns1", Name: "cm1"})

	pods := tr.RecentByResource("pod", 10)
	assert.Len(t, pods, 2)
	assert.Equal(t, "p2", pods[0].Name)
	assert.Equal(t, "p1", pods[1].Name)

	limited := tr.RecentByResource("pod", 1)
	assert.Len(t, limited, 1)
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

func TestRecordConcurrency(t *testing.T) {
	tr := NewChangeTracker(100)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			tr.Record(Change{Resource: "pod", Name: "p"})
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 50; i++ {
			tr.Record(Change{Resource: "node", Name: "n"})
		}
		done <- struct{}{}
	}()
	<-done
	<-done

	recent := tr.Recent(200)
	assert.Equal(t, 100, len(recent))
}
