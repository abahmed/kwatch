package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// fixedVersion is an informer whose resource version never moves: a
// kind nothing changes.
type fixedVersion string

func (v fixedVersion) LastSyncResourceVersion() string { return string(v) }

// answeredFixture is a Source with one synced watch of storage classes,
// whose resource version stays put, and a count of answered lists.
type answeredFixture struct {
	source   *Source
	watch    *watchState
	now      time.Time
	answered uint64
}

func newAnsweredFixture() *answeredFixture {
	f := &answeredFixture{now: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)}
	resource := Resource{Group: "storage.k8s.io", Name: "storageclasses"}
	f.source = &Source{cfg: SourceConfig{
		Now:                 func() time.Time { return f.now },
		OptionalSyncTimeout: 2 * time.Minute,
		Answered: func(group, name string) uint64 {
			if group != resource.Group || name != resource.Name {
				return 0
			}
			return f.answered
		},
	}}
	f.watch = &watchState{
		resource:    resource,
		hasSynced:   func() bool { return true },
		lastVersion: f.source.progressOf(resource, fixedVersion("100")),
		now:         func() time.Time { return f.now },
	}
	f.source.watches = []*watchState{f.watch}
	return f
}

// After a blip a relist of a kind nothing changes returns the version
// the informer already had. The answered list alone ends the staleness.
func TestStaleEndsWhenAListIsAnsweredAtTheSameVersion(t *testing.T) {
	f := newAnsweredFixture()
	f.watch.record(errConnection)
	f.now = f.now.Add(3 * time.Minute)
	assert.Len(t, f.source.Stale(), 1, "no list got through yet")

	f.answered++
	assert.Empty(t, f.source.Stale())
	assert.True(t, f.source.Fresh(KindStorageClass))
}

// Access refused after the sync and granted again: the first answered
// list makes the kind verifiable, though no object changed.
func TestAccessLostEndsWhenAListIsAnsweredAtTheSameVersion(t *testing.T) {
	f := newAnsweredFixture()
	f.watch.record(apierrors.NewForbidden(
		schema.GroupResource{Group: "storage.k8s.io",
			Resource: "storageclasses"}, "", nil))
	assert.True(t, f.watch.accessLost())

	f.answered++
	assert.False(t, f.watch.accessLost())
}

// Without a count the resource version is the mark, as before.
func TestProgressIsTheResourceVersionWithoutACount(t *testing.T) {
	s := &Source{}
	mark := s.progressOf(Resource{Name: "pods"}, fixedVersion("7"))
	assert.Equal(t, "7", mark())
}
