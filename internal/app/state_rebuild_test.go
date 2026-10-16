package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/abahmed/kwatch/internal/storage"
)

// countingOpener fails with err for the first failures calls, then opens
// the real file, like a previous generation that lets go of the lock.
type countingOpener struct {
	failures int
	err      error
	calls    int
}

func (o *countingOpener) open(
	path string, options storage.Options,
) (*storage.Store, error) {
	o.calls++
	if o.calls <= o.failures {
		return nil, o.err
	}
	return storage.Open(path, options)
}

func rebuildStoreDeps(t *testing.T) *serverDeps {
	t.Helper()
	useStateEnv(t)
	return epochDeps(leaseWithTransitions(ptr.To(int32(1))))
}

func TestOpenStoreRetriesALockHeldByThePreviousGeneration(t *testing.T) {
	deps := rebuildStoreDeps(t)
	opener := &countingOpener{failures: 2, err: lockHeld()}
	rec := &sleepRecorder{}
	deps.rebuild, deps.openState = rec.retry(), opener.open
	before := rebuildCounts()

	s, err := openStore(context.Background(), deps)

	require.NoError(t, err)
	defer closeStore(s)
	require.Equal(t, 3, opener.calls)
	require.Len(t, rec.waits, 2)
	require.Same(t, s, deps.openedStore.Load())
	after := rebuildCounts()
	require.Equal(t, before[1]+2, after[1])
	require.Equal(t, before[2], after[2])
}

func TestOpenStoreDoesNotRetryOnAFirstStart(t *testing.T) {
	deps := rebuildStoreDeps(t)
	opener := &countingOpener{failures: 1, err: lockHeld()}
	deps.openState = opener.open

	_, err := openStore(context.Background(), deps)

	require.True(t, storage.IsLockTimeout(err))
	require.Equal(t, 1, opener.calls)
	require.Nil(t, deps.openedStore.Load())
}

func TestOpenStorePermanentFailureOnARebuildFailsOnce(t *testing.T) {
	out := captureKlog(t)
	deps := rebuildStoreDeps(t)
	opener := &countingOpener{failures: 5, err: errors.New("disk full")}
	rec := &sleepRecorder{}
	deps.rebuild, deps.openState = rec.retry(), opener.open
	before := rebuildCounts()

	_, err := openStore(context.Background(), deps)

	require.ErrorContains(t, err, "disk full")
	require.Equal(t, 1, opener.calls)
	require.Empty(t, rec.waits)
	require.Equal(t, before[2]+1, rebuildCounts()[2])
	require.Contains(t, out.String(), "edition rebuild failed")
}

func TestOpenStoreExhaustedRetriesFailWithTheRebuildMessage(t *testing.T) {
	out := captureKlog(t)
	deps := rebuildStoreDeps(t)
	opener := &countingOpener{failures: 100, err: lockHeld()}
	rec := &sleepRecorder{}
	deps.rebuild, deps.openState = rec.retry(), opener.open

	_, err := openStore(context.Background(), deps)

	require.True(t, storage.IsLockTimeout(err))
	require.Equal(t, rebuildRetryBudget, rec.total())
	require.Contains(t, out.String(), "edition rebuild failed")
	require.Contains(t, out.String(), "state store")
}
