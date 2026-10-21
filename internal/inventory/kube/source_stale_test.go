package kube

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// staleFixture is a Source with one synced watch whose clock and resource
// version the test moves.
type staleFixture struct {
	source  *Source
	watch   *watchState
	now     time.Time
	version string
	synced  bool
}

func newStaleFixture() *staleFixture {
	f := &staleFixture{
		now:     time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC),
		version: "100", synced: true,
	}
	f.watch = &watchState{
		resource:    Resource{Name: "pods"},
		required:    true,
		hasSynced:   func() bool { return f.synced },
		lastVersion: func() string { return f.version },
		now:         func() time.Time { return f.now },
	}
	f.source = &Source{
		cfg: SourceConfig{
			Now:                 func() time.Time { return f.now },
			OptionalSyncTimeout: 2 * time.Minute,
		},
		watches: []*watchState{f.watch},
	}
	return f
}

var errConnection = errors.New("dial tcp: connection refused")

func TestSourceStaleListsASyncedWatchThatKeepsFailing(t *testing.T) {
	f := newStaleFixture()
	f.watch.record(errConnection)

	assert.Empty(t, f.source.Stale(), "one failure is not staleness")

	f.now = f.now.Add(f.source.cfg.OptionalSyncTimeout)
	f.watch.record(errConnection)

	assert.Equal(t, []SourceStatus{{
		Resource: "pods", Required: true, Reason: ReasonSyncFailed,
	}}, f.source.Stale())
	assert.Empty(t, f.source.Unavailable(),
		"a synced kind stays available; only health shows the staleness")
}

func TestSourceStaleClearsWhenTheWatchWorksAgain(t *testing.T) {
	f := newStaleFixture()
	f.watch.record(errConnection)
	f.now = f.now.Add(10 * time.Minute)
	assert.Len(t, f.source.Stale(), 1)

	f.version = "101" // a relist got through

	assert.Empty(t, f.source.Stale())
}

func TestSourceStaleRestartsAfterARecoveryWithinTheBound(t *testing.T) {
	f := newStaleFixture()
	f.watch.record(errConnection)
	f.now = f.now.Add(time.Minute)
	f.version = "101" // recovered after a minute
	f.now = f.now.Add(time.Minute)
	f.watch.record(errConnection) // a new, separate failure

	f.now = f.now.Add(90 * time.Second)

	assert.Empty(t, f.source.Stale(),
		"the first failure's minute must not count for the second")
}

func TestSourceStaleIgnoresAnUnsyncedWatchAndRefusedAccess(t *testing.T) {
	f := newStaleFixture()
	f.synced = false
	f.watch.record(errConnection)
	f.now = f.now.Add(time.Hour)
	assert.Empty(t, f.source.Stale(), "unsynced kinds are in Unavailable")

	f = newStaleFixture()
	forbidden := apierrors.NewForbidden(
		schema.GroupResource{Resource: "pods"}, "", errors.New("no"))
	f.watch.record(forbidden)
	f.now = f.now.Add(time.Hour)
	assert.Empty(t, f.source.Stale(), "refused access is accessLost")
	assert.Len(t, f.source.Unavailable(), 1)
}
