package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func probeObs(at time.Time, healthy bool) inventory.Observation {
	attrs := map[string]inventory.Value{
		kube.AttrHealthy:         inventory.Bool(healthy),
		kube.AttrLatencyMS:       inventory.Number(3),
		kube.AttrFailureDuration: inventory.Number(1),
	}
	if !healthy {
		attrs[kube.AttrProbeError] = inventory.Text("HTTP status 500")
	}
	return inventory.Observation{
		Kind: inventory.Observed, Source: "active-probe", At: at,
		Entity:     inventory.CoreID(kube.KindEndpoint, "", "receiver"),
		Attributes: attrs,
	}
}

// A probed endpoint that fails and then answers again resolves after the
// resolve hold.
func TestEngineResolvesRecoveredActiveProbe(t *testing.T) {
	start := time.Date(2026, 10, 3, 3, 0, 0, 0, time.UTC)
	h := newHarness(t, start)
	ctx := context.Background()
	submit := func(healthy bool) {
		h.engine.Submit(ctx, probeObs(h.now, healthy))
	}
	for i := 0; i < 60; i++ {
		submit(false)
		h.run(h.now.Add(time.Second), time.Second)
	}
	if len(h.decisions) != 1 {
		t.Fatalf("got %d decisions, want 1", len(h.decisions))
	}
	for i := 0; i < 400; i++ {
		submit(true)
		h.run(h.now.Add(time.Second), time.Second)
	}
	last := h.decisions[len(h.decisions)-1]
	if last.Action != incident.Resolve {
		t.Fatalf("last action = %v, want resolve (%d decisions)",
			last.Action, len(h.decisions))
	}
}
