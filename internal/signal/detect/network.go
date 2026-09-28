package detect

import (
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// DefaultNoEndpoints is how long a Service may have no ready endpoints;
// rolling restarts briefly empty small Services.
const DefaultNoEndpoints = time.Minute

// Service detects Services that select pods but route to none of them.
// The signal is a symptom: its cause is whatever made the pods unready.
type Service struct{}

// Name implements signal.Detector.
func (Service) Name() string { return "service" }

// Kinds implements signal.Detector.
func (Service) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindService}
}

// Detect implements signal.Detector.
func (Service) Detect(ctx signal.Context, e knowledge.Entity) []signal.Signal {
	if text(e, kube.AttrSelector) == "" ||
		text(e, kube.AttrServiceType) == "ExternalName" {
		return nil
	}
	slices := ctx.Model.Related(e.ID, knowledge.Backs, knowledge.Incoming)
	if len(slices) == 0 {
		return nil
	}
	endpoints, ready := 0.0, 0.0
	var since time.Time
	for _, id := range slices {
		slice, ok := ctx.Model.Entity(id)
		if !ok {
			continue
		}
		total, _ := number(slice, kube.AttrEndpoints)
		up, _ := number(slice, kube.AttrEndpointsReady)
		endpoints += total
		ready += up
		if at := valueSince(slice, kube.AttrEndpointsReady); at.After(since) {
			since = at
		}
	}
	// No endpoints at all means nothing is selected (scaled to zero on
	// purpose); unready endpoints mean selected pods are failing.
	if endpoints == 0 || ready > 0 {
		return nil
	}
	if !sustained(ctx, since, DefaultNoEndpoints) {
		return nil
	}
	return []signal.Signal{{
		Reason: constant.ReasonServiceNoEndpoints, Severity: signal.Critical,
		Since: since, Symptom: true,
		Summary: "Service has no ready backends; traffic to it fails",
	}}
}
