package delivery

import (
	"testing"

	"github.com/abahmed/kwatch/internal/metrics"
)

func TestStoppedManagerCountsDroppedNotifications(t *testing.T) {
	a := &Manager{state: stateStopped}
	before := metrics.DefaultRegistry().NotificationsDropped.Load()
	a.Notify("after stop")
	after := metrics.DefaultRegistry().NotificationsDropped.Load()
	if after != before+1 {
		t.Fatalf("dropped counter %d -> %d, want +1", before, after)
	}
}
