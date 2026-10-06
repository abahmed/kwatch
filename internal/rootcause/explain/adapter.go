package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// NewSnapshot builds the snapshot the application solves: the
// inventory, its active findings by entity, the kube read-time links,
// the change sets with their outcomes and the workload baselines.
func NewSnapshot(
	model inventory.HistoryReader,
	findings map[inventory.EntityID][]detection.Finding,
	synced func(inventory.Kind) bool, now time.Time,
) Snapshot {
	changes := HistoryChanges{History: model}
	return Snapshot{
		Model: model, Findings: findings,
		Links:   KubeLinks{Reader: model},
		Changes: changes, Outcomes: changes,
		Baseline: WorkloadBaselines{History: model, Now: now},
		Synced:   synced, Now: now,
	}
}

// Record turns a solved cause into the record an incident keeps and
// messages are written from: its row and mode, the root's own unhealthy
// findings that are not symptoms, the latest blamed change (see
// BlamedChanges), for a rollout the revision a rollback returns to, and
// each contribution as a proof.
func (s Snapshot) Record(c Cause) rootcause.CauseRecord {
	out := rootcause.CauseRecord{
		Rule: c.Row, Mode: c.Mode, Root: c.Root, Chain: c.Chain,
		Summary: c.Summary, Score: c.Confidence, Began: c.Began,
	}
	out.Hops, out.Beyond = recordHops(c.Hops), recordHops(c.Beyond)
	for _, f := range s.Findings[c.Root] {
		// A symptom finding ("1 of 2 replicas are ready") restates
		// the failures the cause explains; it does not describe it.
		if unhealthy(f) && !f.Symptom {
			out.RootFindings = append(out.RootFindings, f)
		}
	}
	if blamed := s.BlamedChanges(c); len(blamed) > 0 {
		change := blamed[len(blamed)-1]
		out.Change = &change
		out.RollbackRevision = rollbackRevision(s.Model, c.Covers)
	}
	for _, e := range c.Contributions {
		out.Proof = append(out.Proof, rootcause.Proof{
			Code: e.Code, Count: e.Count, Total: e.Total, Fields: e.Fields,
			Edits: e.Edits, Text: e.Text, Weight: abs(e.Weight), Supports: e.Weight > 0,
		})
	}
	return out
}

// recordHops keeps what a message says about each hop of a chain.
func recordHops(hops []Hop) []rootcause.Hop {
	var out []rootcause.Hop
	for _, h := range hops {
		out = append(out, rootcause.Hop{Entity: h.Entity, Began: h.Began})
	}
	return out
}
