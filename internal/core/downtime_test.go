package core

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func modelWithDeployment(t *testing.T, replicas int32) *knowledge.Model {
	t.Helper()
	m := knowledge.NewModel(knowledge.Options{})
	d, _ := deployment("orders")
	d.Spec.Replicas = &replicas
	tr := kube.NewTranslator(kube.DeploymentSchema())
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	for _, f := range tr.Added(d, true, now) {
		if _, err := m.Apply(f); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func TestFingerprintsCoverTrackedObjects(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	m := modelWithDeployment(t, 2)

	got := Fingerprints(m, now)

	if got[snapshotTimeKey] != now.Format(time.RFC3339) {
		t.Errorf("snapshot time = %v", got[snapshotTimeKey])
	}
	if len(got) != 2 {
		t.Fatalf("fingerprints = %v, want snapshot + deployment", got)
	}
}

func TestDowntimeChangesReportsOnlyDifferences(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	before := modelWithDeployment(t, 2)
	saved := map[string]string{}
	for k, v := range Fingerprints(before, now.Add(-time.Hour)) {
		saved[k] = v.(string)
	}

	same := DowntimeChanges(before, saved, now)
	after := modelWithDeployment(t, 5)
	facts := DowntimeChanges(after, saved, now)

	if len(same) != 0 {
		t.Fatalf("unchanged model produced %v", same)
	}
	if len(facts) != 1 {
		t.Fatalf("facts = %d, want 1", len(facts))
	}
	c := facts[0].Change
	if c.Actor != DowntimeActor {
		t.Errorf("actor = %q", c.Actor)
	}
	if !c.At.Equal(now.Add(-time.Hour)) {
		t.Errorf("change dated %v, want the snapshot time", c.At)
	}
	if len(c.Fields) != 1 || c.Fields[0].Path != kube.AttrReplicas ||
		c.Fields[0].Before != "2" || c.Fields[0].After != "5" {
		t.Errorf("fields = %+v", c.Fields)
	}
}

func TestDowntimeChangesIgnoresUnknownAndBadKeys(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	m := modelWithDeployment(t, 2)

	facts := DowntimeChanges(m, map[string]string{"junk": "x"}, now)

	if len(facts) != 0 {
		t.Fatalf("new objects are not downtime changes: %v", facts)
	}
}

func TestDowntimePathMapsTemplateHash(t *testing.T) {
	if got := downtimePath(kube.AttrTemplateHash); got != "spec.template" {
		t.Errorf("path = %q", got)
	}
	if got := downtimePath("x"); got != "x" {
		t.Errorf("path = %q", got)
	}
}

func TestFingerprintFieldsListsChangedAttributes(t *testing.T) {
	fields := fingerprintFields("a=1;b=2", "a=1;b=3;c=4")

	if len(fields) != 2 || fields[0].Path != "b" ||
		fields[1].Path != "c" || fields[1].Before != "" {
		t.Fatalf("fields = %+v", fields)
	}
}

func TestEngineReconcileDowntimeSubmitsChanges(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, (&sinkLog{}).sink,
		func(d *Dependencies) { d.Model = modelWithDeployment(t, 5) })
	e.saved = map[string]string{}
	for k, v := range Fingerprints(modelWithDeployment(t, 2), now) {
		e.saved[k] = v.(string)
	}

	e.reconcileDowntime()

	if len(e.pending) != 1 || e.saved != nil {
		t.Fatalf("pending = %d saved = %v", len(e.pending), e.saved)
	}
}
