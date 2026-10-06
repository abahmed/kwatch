package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func TestOccurrenceDescribe(t *testing.T) {
	opened := time.Date(2026, 9, 29, 14, 2, 0, 0, time.UTC)
	tests := []struct {
		fix  Fix
		want string
	}{
		{FixRollback, "same as Tue 14:02, fixed by rollback"},
		{FixConfig, "same as Tue 14:02, fixed by config fix"},
		{FixNone, "same as Tue 14:02, recovered without change"},
		{"", "same as Tue 14:02"},
	}
	for _, tt := range tests {
		t.Run(string(tt.fix), func(t *testing.T) {
			o := Occurrence{Opened: opened, Fix: tt.fix}
			assert.Equal(t, tt.want, o.Describe())
		})
	}
}

// failOnce raises s at from, clears it at clearAt and ticks until the
// incident of s, rooted at its own entity, resolved.
func failOnce(r *rig, s detection.Finding, from, clearAt time.Duration) {
	r.raise(at(from), s)
	for now := from; now <= from+time.Hour; now += 10 * time.Second {
		if now == clearAt {
			r.clear(at(now), s)
		}
		r.tick(at(now))
		if now > clearAt && r.of(s.Entity).State == Resolved {
			return
		}
	}
	r.t.Fatalf("incident of %s never resolved", s.Entity)
}

func (r *rig) observe(id inventory.EntityID) {
	r.t.Helper()
	_, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: t0, Entity: id,
	})
	require.NoError(r.t, err)
}

func (r *rig) change(id inventory.EntityID, when time.Duration,
	change inventory.Change) {
	r.t.Helper()
	change.At = at(when)
	_, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Changed, Source: "test", At: change.At,
		Entity: id, Change: change,
	})
	require.NoError(r.t, err)
}

func image(before, after string) inventory.Change {
	return inventory.Change{Fields: []inventory.FieldChange{{
		Path: "containers[web].image", Before: before, After: after,
	}}}
}

func TestRecurrenceRemembersHowEachOccurrenceEnded(t *testing.T) {
	tests := []struct {
		name  string
		setup func(r *rig, pod inventory.EntityID)
		want  Fix
	}{
		{"recovered without change", func(*rig, inventory.EntityID) {},
			FixNone},
		{"rollback", func(r *rig, pod inventory.EntityID) {
			r.change(pod, -30*time.Second, image("web:v1", "web:v2"))
			r.change(pod, 3*time.Minute, image("web:v2", "web:v1"))
		}, FixRollback},
		{"config fix", func(r *rig, pod inventory.EntityID) {
			config := entity(kube.KindConfigMap, "web-config")
			r.observe(config)
			r.relate(pod, inventory.References, config)
			r.change(config, 3*time.Minute, inventory.Change{
				Fields: []inventory.FieldChange{{Path: "data.url",
					After: "changed"}}})
		}, FixConfig},
		{"other change", func(r *rig, pod inventory.EntityID) {
			r.change(pod, 3*time.Minute, inventory.Change{
				Fields: []inventory.FieldChange{{Path: "spec.template"}}})
		}, FixChange},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t, Config{})
			web := podSig("web")
			web.Mode = "CrashLoop"
			r.observe(web.Entity)
			tt.setup(r, web.Entity)
			failOnce(r, web, 0, 4*time.Minute)
			first := r.of(web.Entity)
			assert.Equal(t, tt.want, first.Fix)

			// Past RepageWindow: an earlier return would re-open the
			// same incident instead.
			r.raise(at(3*time.Hour), web)
			again := r.of(web.Entity)
			require.NotEqual(t, first.ID, again.ID)
			last, ok := again.LastOccurrence()
			require.True(t, ok)
			assert.Equal(t, Occurrence{Mode: "CrashLoop",
				Modes: []detection.Mode{"CrashLoop"}, Opened: at(0),
				Resolved: first.Resolved, Fix: tt.want, Heard: true}, last)
			assert.Positive(t, last.Duration())
		})
	}
}

func TestRecurrenceNodeReplaced(t *testing.T) {
	r := newRig(t, Config{})
	node := sig(inventory.CoreID(kube.KindNode, "", "n1"),
		"NotReady", detection.Critical)
	r.observe(node.Entity)
	r.raise(at(0), node)
	_, err := r.model.Apply(inventory.Observation{
		Kind: inventory.Gone, Source: "test", At: at(time.Minute),
		Entity: node.Entity,
	})
	require.NoError(t, err)
	r.clear(at(2*time.Minute), node)
	for now := 2 * time.Minute; now <= time.Hour; now += 10 * time.Second {
		r.tick(at(now))
	}
	assert.Equal(t, FixNodeReplaced, r.of(node.Entity).Fix)
}

func TestRecurrenceHistoryIsBoundedAndPersisted(t *testing.T) {
	var history []Occurrence
	for i := 0; i < maxHistory+3; i++ {
		history = appendHistory(history, Occurrence{
			Opened: at(time.Duration(i) * time.Hour)})
	}
	require.Len(t, history, maxHistory)
	assert.Equal(t, at(3*time.Hour), history[0].Opened)

	r := newRig(t, Config{})
	web := podSig("web")
	failOnce(r, web, 0, time.Minute)
	r.raise(at(2*time.Hour), web)
	records := r.m.Export()
	restored := newRig(t, Config{})
	restored.m.Restore(records, time.Time{})
	assert.Len(t, restored.of(web.Entity).History, 1)
}

func TestLastOccurrenceMatchesMode(t *testing.T) {
	p := Incident{Mode: "OOMKilled", History: []Occurrence{
		{Mode: "OOMKilled", Opened: at(0)},
		{Mode: "CrashLoop", Opened: at(time.Hour)},
	}}
	last, ok := p.LastOccurrence()
	require.True(t, ok)
	assert.Equal(t, at(0), last.Opened)
	_, ok = (&Incident{Mode: "ImagePull", History: p.History}).
		LastOccurrence()
	assert.False(t, ok)
}

func TestResolveKeepsTheFixingChange(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.observe(web.Entity)
	r.change(web.Entity, -30*time.Second, image("web:v1", "web:v2"))
	rollback := image("web:v2", "web:v1")
	rollback.Actor = "alice"
	r.change(web.Entity, 3*time.Minute, rollback)
	failOnce(r, web, 0, 4*time.Minute)

	got := r.of(web.Entity).FixedBy
	require.NotNil(t, got, "the rollback fixed it")
	assert.Equal(t, "alice", got.Actor)
	assert.Equal(t, at(3*time.Minute), got.At)
}

func TestResolveWithoutChangeHasNoFixingChange(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.observe(web.Entity)
	failOnce(r, web, 0, 4*time.Minute)
	assert.Nil(t, r.of(web.Entity).FixedBy)
}

func TestTriggerOfClassifiesCauses(t *testing.T) {
	node := entity(kube.KindNode, "n1")
	for name, c := range map[string]struct {
		cause *rootcause.CauseRecord
		want  string
	}{
		"none":    {nil, ""},
		"rollout": {&rootcause.CauseRecord{Rule: "rollout"}, TriggerRollout},
		"config change": {&rootcause.CauseRecord{
			Rule:   "configmap-missing-or-changed",
			Change: &inventory.Change{}}, TriggerConfig},
		"config missing": {&rootcause.CauseRecord{
			Rule: "configmap-missing-or-changed"}, ""},
		"node": {&rootcause.CauseRecord{Rule: "node-not-ready",
			Root: node}, TriggerNode},
		"other": {&rootcause.CauseRecord{Rule: "webhook-rejects"}, ""},
	} {
		if got := TriggerOf(c.cause); got != c.want {
			t.Errorf("%s: trigger = %q, want %q", name, got, c.want)
		}
	}
}
