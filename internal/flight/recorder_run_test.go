package flight

import (
	"context"
	"testing"
	"time"
)

// A ring with no store panics on Append. Run must contain it: the last
// batch is lost, but the process lives and Run returns with its context.
func TestRecorderRunSurvivesAPanickingFlush(t *testing.T) {
	rec, err := NewRecorder(Ring{}, "salt", newClock().Now)
	if err != nil {
		t.Fatal(err)
	}
	rec.record(numbered(0, 3))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		rec.Run(ctx)
	}()

	cancel() // Run flushes once more as it stops, and that flush panics

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after its context ended")
	}
}
