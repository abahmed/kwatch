package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// runSaver starts Run with a tick that never fires, so any write comes from
// Wake alone. The returned stop function cancels and waits for Run.
func runSaver(t *testing.T, saver *threadSaver) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		saver.Run(ctx, make(chan time.Time))
	}()
	return func() {
		cancel()
		<-done
	}
}

func awaitWrite(t *testing.T, saver *threadSaver) {
	t.Helper()
	select {
	case <-saver.Written():
	case <-time.After(10 * time.Second):
		t.Fatal("thread saver did not write after Wake")
	}
}

// The crash window: a provider posts the first message, records its thread,
// and the process dies before the periodic save. The next leader restores
// from the store and must find the thread.
func TestThreadSaverFirstThreadSurvivesCrashBeforeTick(t *testing.T) {
	disk := threadDisk(t)
	src := &fakeThreads{}
	saver := newThreadSaver(src, disk)
	stop := runSaver(t, saver)
	defer stop()

	src.set(sampleThreads)
	saver.Wake()
	awaitWrite(t, saver)

	// No Flush and no tick: this is the state a crash would leave behind.
	next := &fakeThreads{}
	newThreadSaver(next, disk).Restore()
	require.Equal(t, []map[string]map[string]string{sampleThreads},
		next.restored)
}

func TestThreadSaverWakeWithoutNewsDoesNotWrite(t *testing.T) {
	disk := threadDisk(t)
	src := &fakeThreads{threads: sampleThreads}
	saver := newThreadSaver(src, disk)
	require.NoError(t, saver.Save(context.Background()))
	<-saver.Written()
	stop := runSaver(t, saver)

	require.NoError(t, disk.putThreads("marker"))
	saver.Wake()
	stop()

	var marker string
	_, err := disk.getThreads(&marker)
	require.NoError(t, err)
	require.Equal(t, "marker", marker, "unchanged snapshot must not write")
}

func TestThreadSaverWakeIsNonBlockingAndStopsAfterClose(t *testing.T) {
	disk := threadDisk(t)
	saver := newThreadSaver(&fakeThreads{threads: sampleThreads}, disk)
	for i := 0; i < 100; i++ {
		saver.Wake()
	}
	saver.Close()
	saver.Wake()

	stop := runSaver(t, saver)
	stop()
	var rec threadRecord
	found, err := disk.getThreads(&rec)
	require.NoError(t, err)
	require.False(t, found, "no write may follow Close")
}

func TestThreadWakeReachesAttachedSaverOnly(t *testing.T) {
	var wake *threadWake
	require.NotPanics(t, func() {
		wake.Notify()
		wake.attach(nil)
		wake.detach()
	})

	wake = &threadWake{}
	saver := newThreadSaver(&fakeThreads{}, threadDisk(t))
	wake.Notify()
	require.Empty(t, saver.wake)

	wake.attach(saver)
	wake.Notify()
	require.Len(t, saver.wake, 1)

	<-saver.wake
	wake.detach()
	wake.Notify()
	require.Empty(t, saver.wake)
}
