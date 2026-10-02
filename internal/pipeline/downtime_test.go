package pipeline

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func modelWithDeployment(t *testing.T, replicas int32) *inventory.Model {
	t.Helper()
	m := inventory.NewModel(inventory.Options{})
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
	if len(got) != 2+len(fingerprintAttributes) {
		t.Fatalf("fingerprints = %v, want snapshot, kind times and "+
			"deployment", got)
	}
	if got[kindTimePrefix+string(kube.KindSecret)] != got[snapshotTimeKey] {
		t.Errorf("a kind without objects is still covered: %v", got)
	}
}

func TestDowntimeChangesReportsOnlyDifferences(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	before := modelWithDeployment(t, 2)
	saved := map[string]string{}
	for k, v := range Fingerprints(before, now.Add(-time.Hour)) {
		saved[k] = v.(string)
	}

	same := DowntimeChanges(before, saved, nil, now)
	after := modelWithDeployment(t, 5)
	observations := DowntimeChanges(after, saved, nil, now)

	if len(same) != 0 {
		t.Fatalf("unchanged model produced %v", same)
	}
	if len(observations) != 1 {
		t.Fatalf("observations = %d, want 1", len(observations))
	}
	c := observations[0].Change
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

// Without a snapshot time for its kind, an object with no saved
// fingerprint was not necessarily created while kwatch was down: the
// first run, and a state file from before kinds were covered, report
// nothing.
func TestDowntimeChangesFirstRunReportsNoCreation(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	m := modelWithDeployment(t, 2)

	for name, saved := range map[string]map[string]string{
		"empty":        {},
		"bad keys":     {"junk": "x"},
		"no kind time": {snapshotTimeKey: now.Format(time.RFC3339)},
	} {
		if got := DowntimeChanges(m, saved, nil, now); len(got) != 0 {
			t.Errorf("%s: reported %+v", name, got)
		}
	}
}

// An object of a covered kind without a saved fingerprint was created
// while kwatch was down; the change is dated at the snapshot.
func TestDowntimeChangesReportsCreation(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	snapshot := now.Add(-time.Hour)
	saved := savedFingerprints(t, inventory.NewModel(inventory.Options{}),
		snapshot)
	m := modelWithDeployment(t, 2)

	observations := DowntimeChanges(m, saved, nil, now)

	if len(observations) != 1 {
		t.Fatalf("observations = %+v, want one creation", observations)
	}
	c := observations[0].Change
	if !c.Created || c.Deleted || len(c.Fields) != 0 ||
		c.Actor != DowntimeActor || !c.At.Equal(snapshot) {
		t.Errorf("change = %+v", c)
	}
}

// A saved fingerprint whose object is gone after the initial list was
// deleted while kwatch was down. The change is recorded on the absent
// entity, as a live deletion is, so root-cause rules can still find it.
func TestDowntimeChangesReportsDeletion(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	snapshot := now.Add(-time.Hour)
	before := modelWithDeployment(t, 2)
	applyAll(t, before, kube.ConfigMapSchema{}, configMap("cfg"), now)
	saved := savedFingerprints(t, before, snapshot)
	cfg := before.Entities(kube.KindConfigMap)[0]
	after := modelWithDeployment(t, 2)

	observations := DowntimeChanges(after, saved, nil, now)

	if len(observations) != 1 || observations[0].Entity != cfg {
		t.Fatalf("observations = %+v, want the deletion of %s",
			observations, cfg)
	}
	c := observations[0].Change
	if !c.Deleted || c.Created || c.Actor != DowntimeActor ||
		!c.At.Equal(snapshot) {
		t.Errorf("change = %+v", c)
	}
	if _, err := after.Apply(observations[0]); err != nil {
		t.Fatal(err)
	}
	changes := after.Changes(cfg, snapshot)
	if after.Exists(cfg) || len(changes) != 1 || !changes[0].Deleted {
		t.Errorf("exists = %v changes = %+v", after.Exists(cfg), changes)
	}
}

// A kind that has not synced is never compared: a partial list would
// read as deletions and creations.
func TestDowntimeChangesSkipsUnsyncedKind(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	before := modelWithDeployment(t, 2)
	applyAll(t, before, kube.ConfigMapSchema{}, configMap("cfg"), now)
	saved := savedFingerprints(t, before, now.Add(-time.Hour))
	after := modelWithDeployment(t, 5)
	applyAll(t, after, kube.ConfigMapSchema{}, configMap("other"), now)
	unsynced := func(k inventory.Kind) bool { return k != kube.KindConfigMap }

	observations := DowntimeChanges(after, saved, unsynced, now)

	if len(observations) != 1 ||
		observations[0].Entity.Kind != kube.KindDeployment {
		t.Fatalf("observations = %+v, want only the deployment edit",
			observations)
	}
}

func applyAll(
	t *testing.T, m *inventory.Model, schema kube.Schema, obj any,
	now time.Time,
) {
	t.Helper()
	for _, o := range kube.NewTranslator(schema).Added(obj, true, now) {
		if _, err := m.Apply(o); err != nil {
			t.Fatal(err)
		}
	}
}

func configMap(name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "shop",
			UID: types.UID(name)},
		Data: map[string]string{"LISTEN_PORT": "8080"},
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

func TestEngineReconcileDowntimeAppliesChangesDirectly(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, (&sinkLog{}).sink,
		func(d *Dependencies) { d.Model = modelWithDeployment(t, 5) })
	e.storage.saved = map[string]string{}
	for k, v := range Fingerprints(modelWithDeployment(t, 2), now) {
		e.storage.saved[k] = v.(string)
	}

	e.reconcileDowntime()

	if len(e.inbox.pending) != 0 || len(e.dirty) == 0 || e.storage.saved != nil {
		t.Fatalf("pending = %d dirty = %d saved = %v",
			len(e.inbox.pending), len(e.dirty), e.storage.saved)
	}
}
