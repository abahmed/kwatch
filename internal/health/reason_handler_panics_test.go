package health

import (
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

// The source reporting publishes handler_panics for a resource whose
// event handler keeps panicking. It is part of the vocabulary: health must
// not turn it into the generic component_failed.
func TestSetComponentStatusKeepsHandlerPanics(t *testing.T) {
	server := NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{})

	server.SetComponentStatus("source-pods", "degraded", "handler_panics",
		false)

	if got := server.ComponentStatuses()["source-pods"].Reason; got !=
		"handler_panics" {
		t.Errorf("reason = %q, want handler_panics", got)
	}
}
