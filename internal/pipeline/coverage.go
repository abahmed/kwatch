package pipeline

import (
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// The coverage check is explained in coverage/doc.go. This file is its
// engine side: it hands back what the check finds.

// checkCoverage finds failing workloads that no live incident speaks for
// and hands their findings back to the incident manager.
func (e *Engine) checkCoverage(now time.Time) {
	w := &e.coverage
	if !w.Due(now) {
		return
	}
	failing := w.FailingWorkloads(e.deps.Model, now)
	if len(failing) == 0 {
		return
	}
	covered := e.deps.Incidents.CoveredWorkloads(now)
	var lost, dirty []inventory.EntityID
	for _, id := range failing {
		if covered[id] || !w.Settled(id, now) {
			continue
		}
		own := e.findingEntities(id)
		if len(own) == 0 {
			continue
		}
		w.HandBack(id, now)
		lost = append(lost, id)
		dirty = append(dirty, own...)
	}
	if len(lost) == 0 {
		return
	}
	klog.InfoS("pipeline: coverage check found failing workloads no "+
		"incident covers; an incident was lost, opening one",
		"component", "pipeline", "workloads", entityNames(lost))
	snapshot := explain.NewSnapshot(e.deps.Model, e.activeFindings(),
		e.deps.Synced, now)
	e.deps.Incidents.Apply(snapshot, dirty, nil)
	e.announcer.investigateOpened(now)
}

// findingEntities lists the workload, its pods and their containers
// that have an active failure finding.
func (e *Engine) findingEntities(
	workload inventory.EntityID,
) []inventory.EntityID {
	candidates := []inventory.EntityID{workload}
	for _, pod := range rootcause.OwnedPods(e.deps.Model, workload) {
		candidates = append(candidates, pod)
		candidates = append(candidates, e.deps.Model.Related(
			pod, inventory.PartOf, inventory.Incoming)...)
	}
	var out []inventory.EntityID
	for _, id := range candidates {
		if e.hasFailure(id) {
			out = append(out, id)
		}
	}
	return out
}

// hasFailure reports an active finding of id that is a failure, not a
// configuration risk.
func (e *Engine) hasFailure(id inventory.EntityID) bool {
	for _, f := range e.findings[id] {
		if !f.Advisory {
			return true
		}
	}
	return false
}

func entityNames(ids []inventory.EntityID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}
