package metrics

import (
	"strings"
	"testing"
)

func TestDeliveryMetricsBoundQueueDepthToConfiguredProviders(t *testing.T) {
	r := &Registry{}
	if body := scrape(t, r); !strings.Contains(body,
		`kwatch_delivery_queue_depth{provider="none"} 0`) {
		t.Fatalf("queue depth family missing before configuration:\n%s",
			body)
	}
	r.Delivery.ConfigureQueueProviders([]string{"slack", "pagerduty"})
	r.Delivery.SetQueueDepth("slack", 7)
	r.Delivery.SetQueueDepth("unconfigured", 9)
	r.Delivery.OutboxDepth.Store(3)
	r.Delivery.Deferred.Add(2)

	body := scrape(t, r)
	for _, want := range []string{
		`kwatch_delivery_queue_depth{provider="slack"} 7`,
		`kwatch_delivery_queue_depth{provider="pagerduty"} 0`,
		"kwatch_delivery_outbox_depth 3",
		"kwatch_delivery_deferred_total 2",
		"kwatch_tracker_untracked_total 0",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "unconfigured") {
		t.Error("an unconfigured provider became a label")
	}
	if got := r.Delivery.QueueDepth("slack"); got != 7 {
		t.Errorf("QueueDepth = %d, want 7", got)
	}
}
