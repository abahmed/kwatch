package incident

import (
	"sort"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

var t0 = time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)

func at(d time.Duration) time.Time { return t0.Add(d) }

// stubExplainer solves every dirty entity with findings as an area of
// its own, explained by the cause set for it, if any.
type stubExplainer struct {
	causes     map[inventory.EntityID]explain.Cause
	unverified map[inventory.EntityID][]string
	// rejected are the candidates dropped for a failure's area.
	rejected map[inventory.EntityID][]explain.Rejection
	// last is the snapshot of the latest Solve.
	last explain.Snapshot
}

func (e *stubExplainer) Solve(
	s explain.Snapshot, dirty []inventory.EntityID,
) []explain.Area {
	e.last = s
	seen := map[inventory.EntityID]bool{}
	var out []explain.Area
	for _, id := range dirty {
		if seen[id] || len(s.Findings[id]) == 0 {
			continue
		}
		seen[id] = true
		area := explain.Area{Failures: []inventory.EntityID{id},
			Unverified: e.unverified[id],
			Trace:      explain.Trace{Rejected: e.rejected[id]}}
		if c, ok := e.causes[id]; ok {
			c.Covers = []inventory.EntityID{id}
			area.Causes = []explain.Cause{c}
		} else {
			area.Unexplained = area.Failures
		}
		out = append(out, area)
	}
	return out
}

type rig struct {
	t     *testing.T
	model *inventory.Model
	rule  *stubExplainer
	// sigs are extra active findings the snapshot holds, such as the
	// findings of a cause's root.
	sigs map[inventory.EntityID][]detection.Finding
	// active are the findings raised and not cleared.
	active map[detection.Key]detection.Finding
	m      *Manager
}

// testNonce makes rig IDs deterministic.
const testNonce = "7f3a"

func newRig(t *testing.T, cfg Config) *rig {
	t.Helper()
	if cfg.IDNonce == "" {
		cfg.IDNonce = testNonce
	}
	rule := &stubExplainer{
		causes:     map[inventory.EntityID]explain.Cause{},
		unverified: map[inventory.EntityID][]string{},
		rejected:   map[inventory.EntityID][]explain.Rejection{},
	}
	return &rig{
		t:      t,
		model:  inventory.NewModel(inventory.Options{}),
		rule:   rule,
		sigs:   map[inventory.EntityID][]detection.Finding{},
		active: map[detection.Key]detection.Finding{},
		m:      NewManager(cfg, rule),
	}
}

func (r *rig) relate(
	from inventory.EntityID, rel inventory.RelationType,
	to ...inventory.EntityID,
) {
	r.t.Helper()
	_, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Related, Source: "test", At: t0,
		Entity: from, Relation: rel, Targets: to,
	})
	if err != nil {
		r.t.Fatal(err)
	}
}

// cause makes root, with summary, the cause of symptom's failure.
func (r *rig) cause(
	symptom, root inventory.EntityID, summary string,
) explain.Cause {
	c := explain.Cause{Root: root, Summary: summary, Confidence: 0.9}
	r.rule.causes[symptom] = c
	return c
}

// snapshot holds the active findings and the extra sigs.
func (r *rig) snapshot(now time.Time) explain.Snapshot {
	findings := map[inventory.EntityID][]detection.Finding{}
	for id, extra := range r.sigs {
		findings[id] = append(findings[id], extra...)
	}
	keys := make([]detection.Key, 0, len(r.active))
	for key := range r.active {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Reason < keys[j].Reason
	})
	for _, key := range keys {
		findings[key.Entity] = append(findings[key.Entity], r.active[key])
	}
	return explain.Snapshot{Model: r.model, Findings: findings, Now: now}
}

func (r *rig) apply(
	now time.Time, kind detection.TransitionKind, sigs ...detection.Finding,
) {
	var ts []detection.Transition
	for _, s := range sigs {
		ts = append(ts, detection.Transition{Kind: kind, Finding: s})
		if kind == detection.Cleared {
			delete(r.active, s.Key())
		} else {
			r.active[s.Key()] = s
		}
	}
	r.m.Apply(r.snapshot(now), nil, ts)
}

func (r *rig) raise(now time.Time, sigs ...detection.Finding) {
	r.apply(now, detection.Raised, sigs...)
}

func (r *rig) clear(now time.Time, sigs ...detection.Finding) {
	r.apply(now, detection.Cleared, sigs...)
}

// only returns the single incident the manager holds.
func (r *rig) only() Incident {
	r.t.Helper()
	records := r.m.Export()
	if len(records) != 1 {
		r.t.Fatalf("want one incident, got %d", len(records))
	}
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	return r.m.incidents[records[0].ID].Snapshot()
}

// of returns a snapshot of root's latest incident.
func (r *rig) of(root inventory.EntityID) Incident {
	r.t.Helper()
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	p := r.m.lookup(root)
	if p == nil {
		r.t.Fatalf("no incident for %s", root)
	}
	return p.Snapshot()
}

// idOf returns the ID of root's latest incident.
func (r *rig) idOf(root inventory.EntityID) string {
	r.t.Helper()
	return r.of(root).ID
}

func (r *rig) tick(now time.Time) []Decision {
	d, _ := r.m.Tick(now)
	return d
}

func entity(kind inventory.Kind, name string) inventory.EntityID {
	return inventory.CoreID(kind, "shop", name)
}

func sig(
	id inventory.EntityID, why string, sev detection.Severity,
) detection.Finding {
	return detection.Finding{
		Entity: id, Reason: why, Severity: sev, Since: t0,
		Summary: why + " on " + id.Name,
	}
}

func podSig(name string) detection.Finding {
	return sig(entity(kube.KindPod, name),
		reasons.CrashLoopBackOff, detection.Warning)
}

func wantAction(t *testing.T, ds []Decision, a Action, why string) {
	t.Helper()
	if len(ds) != 1 || ds[0].Action != a || ds[0].Reason != why {
		t.Fatalf("want one %v (%s), got %+v", a, why, ds)
	}
}

func wantNone(t *testing.T, ds []Decision) {
	t.Helper()
	if len(ds) != 0 {
		t.Fatalf("want no decisions, got %+v", ds)
	}
}
