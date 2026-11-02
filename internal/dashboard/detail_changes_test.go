package dashboard

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// failingAt is an incident on root that started failing at since.
func failingAt(
	id string, root inventory.EntityID, since time.Time,
) incident.Incident {
	f := detection.Finding{Entity: root, Reason: "CrashLoopBackOff",
		Since: since}
	return incident.Incident{ID: id, Root: root, State: incident.Open,
		Tier: incident.Notify, Opened: since,
		Members: map[detection.Key]detection.Finding{f.Key(): f}}
}

func imageChange(at time.Time, before, after string) inventory.Change {
	return inventory.Change{At: at, Actor: "kubectl-set",
		Fields: []inventory.FieldChange{{Path: "containers[api].image",
			Before: before, After: after}}}
}

// changesFixture is a root with a used ConfigMap and a neighbour, each
// changed around an incident that began ten minutes ago.
func changesFixture(t *testing.T) (*testModel, incident.Incident) {
	tm := newTestModel(t)
	root, _ := tm.deployment()
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "app-config")
	other := inventory.CoreID(kube.KindConfigMap, "shop", "other")
	tm.observe(cm, nil)
	tm.observe(other, nil)
	tm.relate(root, inventory.References, cm)
	onset := testEpoch.Add(-10 * time.Minute)
	tm.change(cm, inventory.Change{At: onset.Add(-5 * time.Minute),
		Actor: "argocd", Fields: []inventory.FieldChange{
			{Path: "data.DB_PASSWORD", Before: "h1", After: "h2"}}})
	tm.change(other, inventory.Change{At: onset.Add(-time.Minute),
		Fields: []inventory.FieldChange{{Path: "data.X"}}})
	tm.change(root, imageChange(onset.Add(-2*time.Minute),
		"reg.io/team/api:1", "reg.io/team/api:2"))
	fix := imageChange(onset.Add(5*time.Minute), "reg.io/team/api:2",
		"reg.io/team/api:1")
	tm.change(root, fix)
	p := failingAt("c", root, onset)
	fix.Entity = root
	p.FixedBy = &fix
	return tm, p
}

func TestDetailChangesListsNearChangesBeforeAndDuring(t *testing.T) {
	tm, p := changesFixture(t)
	d := detailOf(t, contextServer(tm.m, true, p), "c")

	if len(d.Changes) != 3 {
		t.Fatalf("changes = %+v", d.Changes)
	}
	before, cm, root := d.Changes[0], d.Changes[1], d.Changes[2]
	if before.Name != "api" || before.When != "before" ||
		before.Summary != "image api:1 → api:2" || before.Fixing {
		t.Errorf("change before = %+v", before)
	}
	if root.Name != "api" || root.When != "during" || !root.Fixing ||
		root.Summary != "image api:2 → api:1" ||
		root.Actor != "kubectl-set" {
		t.Errorf("root change = %+v", root)
	}
	if cm.Name != "app-config" || cm.When != "before" ||
		cm.Summary != "key DB_PASSWORD changed" || cm.Actor != "argocd" {
		t.Errorf("config change = %+v", cm)
	}
	if d.FixedBy == nil || !d.FixedBy.Fixing ||
		d.FixedBy.Summary != root.Summary {
		t.Errorf("fixedBy = %+v", d.FixedBy)
	}
}

func TestDetailChangesMarkTheChangeTheCauseBlames(t *testing.T) {
	tm, p := changesFixture(t)
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "app-config")
	p.Cause = &rootcause.CauseRecord{Root: p.Root,
		Change: &inventory.Change{Entity: cm,
			At: p.Opened.Add(-5 * time.Minute)}}
	d := detailOf(t, contextServer(tm.m, true, p), "c")
	var blamed []string
	for _, c := range d.Changes {
		if c.Cause {
			blamed = append(blamed, c.Name)
		}
	}
	if len(blamed) != 1 || blamed[0] != "app-config" {
		t.Errorf("blamed changes = %q", blamed)
	}
}

func TestDetailChangesNeedTheChangeHistoryEntitlement(t *testing.T) {
	tm, p := changesFixture(t)
	d := detailOf(t, contextServer(tm.m, false, p), "c")
	if d.Changes != nil || d.FixedBy != nil {
		t.Errorf("Community shows changes: %+v %+v", d.Changes, d.FixedBy)
	}
}

func TestDetailChangesAreBoundedAndCounted(t *testing.T) {
	tm := newTestModel(t)
	root, _ := tm.deployment()
	onset := testEpoch.Add(-10 * time.Minute)
	var used []inventory.EntityID
	for i := range maxChanges + 3 {
		cm := inventory.CoreID(kube.KindConfigMap, "shop",
			string(rune('a'+i)))
		tm.observe(cm, nil)
		used = append(used, cm)
		tm.change(cm, inventory.Change{
			At:     onset.Add(-time.Duration(i+1) * time.Minute),
			Fields: []inventory.FieldChange{{Path: "data.K"}}})
	}
	tm.relate(root, inventory.References, used...)
	d := detailOf(t, contextServer(tm.m, true,
		failingAt("c", root, onset)), "c")
	if len(d.Changes) != maxChanges || d.Truncated == nil ||
		d.Truncated.Changes != 3 {
		t.Errorf("changes %d truncated %+v", len(d.Changes), d.Truncated)
	}
}

func TestChangeSummaryWording(t *testing.T) {
	cases := []struct {
		change inventory.Change
		want   string
	}{
		{inventory.Change{Created: true}, "created"},
		{inventory.Change{Deleted: true}, "deleted"},
		{inventory.Change{}, "changed"},
		{inventory.Change{Fields: []inventory.FieldChange{
			{Path: "spec.replicas", Before: "3", After: "1"},
			{Path: "spec.paused"}}}, "replicas 3 → 1 (and 1 more)"},
		{inventory.Change{Fields: []inventory.FieldChange{
			{Path: "spec.containers[web].image", After: "web@sha256:" +
				"0123456789abcdef0123"}}},
			"image set to web@sha256:0123456789ab"},
		{inventory.Change{Fields: []inventory.FieldChange{
			{Path: "spec.selector"}}}, "spec.selector changed"},
	}
	for _, c := range cases {
		if got := changeSummary(c.change); got != c.want {
			t.Errorf("summary = %q, want %q", got, c.want)
		}
	}
}
