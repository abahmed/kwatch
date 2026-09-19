package app

import (
	"context"
	"time"
)

const progressHeartbeatInterval = 10 * time.Second

// startProgressHeartbeat keeps an idle but healthy periodic component visible
// to the supervisor. The returned function waits for the heartbeat goroutine
// to stop, so it cannot outlive its owning component.
func startProgressHeartbeat(
	ctx context.Context,
	progress func(),
) func() {
	if progress == nil {
		return func() {}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(progressHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				progress()
			case <-ctx.Done():
				return
			case <-stop:
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}
