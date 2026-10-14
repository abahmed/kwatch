package pipeline

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// probeDetector reports one critical finding for every pod, with the
// evidence a test sets.
type probeDetector struct{ evidence []detection.Evidence }

func (*probeDetector) Name() string { return "probe" }

func (*probeDetector) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindPod}
}

func (p *probeDetector) Detect(
	_ detection.Context, e inventory.Entity,
) []detection.Finding {
	return []detection.Finding{{
		Entity: e.ID, Reason: "CrashLoopBackOff",
		Severity: detection.Critical, Summary: "app is crashing",
		Since:    time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		Evidence: p.evidence,
	}}
}

// A finding whose evidence changes after its announcement refreshes the
// copy the incident holds, and sends no message by itself.
func TestEngineRefreshedEvidenceSendsNothingAndIsFresh(t *testing.T) {
	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	probe := &probeDetector{evidence: []detection.Evidence{
		{Label: "last log", Value: "starting up"}}}
	h := newHarnessWith(t, start, func(d *Dependencies) {
		d.Detectors = detection.NewRegistry(nil, probe)
	})
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("reports")
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	crashing := crashingPod("reports-a", rs.Name, "n1", start)
	h.add(kube.PodSchema{}, crashing)
	h.run(start.Add(10*time.Minute), 10*time.Second)
	if len(h.messages) != 1 {
		t.Fatalf("messages = %d, want the announcement only",
			len(h.messages))
	}

	probe.evidence = []detection.Evidence{
		{Label: "last log", Value: "database password rejected"}}
	next := crashing.DeepCopy()
	next.ResourceVersion = "2"
	h.engine.Submit(ctxBackground(),
		kube.NewTranslator(kube.PodSchema{}).Updated(crashing, next, h.now)...)
	h.run(h.now.Add(2*time.Minute), 10*time.Second)

	if len(h.messages) != 1 {
		t.Fatalf("messages = %d, a refresh must not send one",
			len(h.messages))
	}
	incidents := h.engine.deps.Incidents.Incidents()
	if len(incidents) != 1 {
		t.Fatalf("incidents = %d, want 1", len(incidents))
	}
	for _, f := range incidents[0].Members {
		if got := f.Evidence[0].Value; got != "database password rejected" {
			t.Fatalf("member evidence = %q, want the fresh line", got)
		}
	}
}
