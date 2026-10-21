package app

import (
	"sync"
	"testing"
	"time"
)

// hasMonotonic reports whether t still carries a monotonic reading:
// Round(0) strips it, so a time that changes under Round(0) had one.
func hasMonotonic(t time.Time) bool { return t != t.Round(0) }

func TestComponentProgressKeepsTheMonotonicReading(t *testing.T) {
	progress := newComponentProgress(time.Now())

	if !hasMonotonic(progress.LastProgress()) {
		t.Error("progress stored the wall clock only; a wall-clock step " +
			"would fake or hide a stall")
	}
	progress.Touch(time.Now())
	if !hasMonotonic(progress.LastProgress()) {
		t.Error("a touched time lost its monotonic reading")
	}
}

func TestComponentProgressStallUsesElapsedMonotonicTime(t *testing.T) {
	supervisor := newComponentSupervisor(time.Now)
	started := time.Now()
	progress := newComponentProgress(started)
	watch := runWatch{stall: time.Hour, startup: time.Hour}

	if supervisor.progressStalled(progress.LastProgress(), started, watch) {
		t.Error("a fresh report must not be a stall")
	}
	// Reports older than the stall deadline stall, whatever the wall
	// clock says: the age is measured on the monotonic reading.
	old := newComponentProgress(started.Add(-2 * time.Hour))
	if !supervisor.progressStalled(old.LastProgress(), started, watch) {
		t.Error("a report older than the stall deadline must stall")
	}
}

func TestComponentProgressIsSafeForConcurrentUse(t *testing.T) {
	progress := newComponentProgress(time.Now())
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				progress.Touch(time.Now())
				_ = progress.LastProgress()
			}
		}()
	}
	wg.Wait()
	if progress.LastProgress().IsZero() {
		t.Error("progress was lost")
	}
}
