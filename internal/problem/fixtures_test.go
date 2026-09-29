package problem

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/reason"
	"github.com/abahmed/kwatch/internal/signal"
)

var t0 = time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)

func at(d time.Duration) time.Time { return t0.Add(d) }

// stubRule explains a symptom entity with a fixed hypothesis.
type stubRule struct {
	causes map[knowledge.EntityID]reason.Hypothesis
}

func (r *stubRule) Name() string { return "stub" }

func (r *stubRule) Explain(
	_ reason.Query, s signal.Signal,
) []reason.Hypothesis {
	if h, ok := r.causes[s.Entity]; ok {
		return []reason.Hypothesis{h}
	}
	return nil
}

type stubSignals map[knowledge.EntityID][]signal.Signal

func (s stubSignals) Active(id knowledge.EntityID) []signal.Signal {
	return s[id]
}

func (s stubSignals) Has(id knowledge.EntityID) bool {
	return len(s[id]) > 0
}

type rig struct {
	t     *testing.T
	model *knowledge.Model
	rule  *stubRule
	sigs  stubSignals
	m     *Manager
}

func newRig(t *testing.T, cfg Config) *rig {
	t.Helper()
	rule := &stubRule{
		causes: map[knowledge.EntityID]reason.Hypothesis{},
	}
	return &rig{
		t:     t,
		model: knowledge.NewModel(knowledge.Options{}),
		rule:  rule,
		sigs:  stubSignals{},
		m:     NewManager(cfg, reason.NewEngine(0, rule)),
	}
}

func (r *rig) relate(
	from knowledge.EntityID, rel knowledge.RelationType,
	to ...knowledge.EntityID,
) {
	r.t.Helper()
	_, err := r.model.Apply(knowledge.Fact{
		Kind: knowledge.Related, Source: "test", At: t0,
		Entity: from, Relation: rel, Targets: to,
	})
	if err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) cause(
	symptom, root knowledge.EntityID, summary string,
) reason.Hypothesis {
	h := reason.Hypothesis{Root: root, Summary: summary, Score: 0.9}
	r.rule.causes[symptom] = h
	return h
}

func (r *rig) query(now time.Time) reason.Query {
	return reason.Query{Model: r.model, Signals: r.sigs, Now: now}
}

func (r *rig) apply(
	now time.Time, kind signal.TransitionKind, sigs ...signal.Signal,
) {
	var ts []signal.Transition
	for _, s := range sigs {
		ts = append(ts, signal.Transition{Kind: kind, Signal: s})
	}
	r.m.Apply(r.query(now), ts)
}

func (r *rig) raise(now time.Time, sigs ...signal.Signal) {
	r.apply(now, signal.Raised, sigs...)
}

func (r *rig) clear(now time.Time, sigs ...signal.Signal) {
	r.apply(now, signal.Cleared, sigs...)
}

// only returns the single problem the manager holds.
func (r *rig) only() Problem {
	r.t.Helper()
	records := r.m.Export()
	if len(records) != 1 {
		r.t.Fatalf("want one problem, got %d", len(records))
	}
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	return r.m.problems[records[0].ID].Snapshot()
}

func (r *rig) tick(now time.Time) []Decision {
	d, _ := r.m.Tick(now)
	return d
}

func entity(kind knowledge.Kind, name string) knowledge.EntityID {
	return knowledge.NewEntityID(kind, "shop", name)
}

func sig(
	id knowledge.EntityID, why string, sev signal.Severity,
) signal.Signal {
	return signal.Signal{
		Entity: id, Reason: why, Severity: sev, Since: t0,
		Summary: why + " on " + id.Name,
	}
}

func podSig(name string) signal.Signal {
	return sig(entity(kube.KindPod, name),
		constant.ReasonCrashLoopBackOff, signal.Warning)
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
