package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const ownedFinalizer = "widgets.example.com/cleanup"

// finalizerFields is the field set of a manager that put finalizers on
// an object: metadata.finalizers is a set keyed `v:"<finalizer>"`.
func finalizerFields(names ...string) *metav1.FieldsV1 {
	set := `".":{}`
	for _, name := range names {
		set += `,"v:\"` + name + `\"":{}`
	}
	return &metav1.FieldsV1{Raw: []byte(
		`{"f:metadata":{"f:finalizers":{` + set + `}}}`)}
}

func deletingWidget(
	entries ...metav1.ManagedFieldsEntry,
) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1", "kind": "Widget",
		"metadata": map[string]any{"name": "w", "namespace": "ops"},
	}}
	stamp := metav1.NewTime(fixedTime())
	u.SetDeletionTimestamp(&stamp)
	u.SetFinalizers([]string{ownedFinalizer, "other.example.com/x"})
	u.SetManagedFields(entries)
	return u
}

// widgetAttributes are the attributes the model gets for the object.
func widgetAttributes(
	t *testing.T, u *unstructured.Unstructured,
) map[string]inventory.Value {
	t.Helper()
	tr := kube.NewTranslator(kube.NewUnstructuredSchema("example.com",
		"Widget"))
	for _, o := range tr.Added(u, true, fixedTime()) {
		if o.Kind == inventory.Observed {
			return o.Attributes
		}
	}
	t.Fatal("no observation")
	return nil
}

func updateEntry(
	manager string, at time.Time, fields *metav1.FieldsV1,
) metav1.ManagedFieldsEntry {
	return metav1.ManagedFieldsEntry{Manager: manager, Time: timePtr(at),
		Operation: metav1.ManagedFieldsOperationUpdate, FieldsV1: fields}
}

func TestFinalizerManagersNameTheOwnerOfEachFinalizer(t *testing.T) {
	at := fixedTime()
	u := deletingWidget(
		updateEntry("widget-ctl", at, finalizerFields(ownedFinalizer)),
		updateEntry("kubectl", at.Add(time.Hour), nil))

	attrs := widgetAttributes(t, u)

	assert.Equal(t, ownedFinalizer+"=widget-ctl",
		attrs[kube.AttrFinalizerManagers].AsText())
}

// The informer transform drops the field sets; the owners must survive it.
func TestFinalizerManagersSurviveTheInformerTransform(t *testing.T) {
	at := fixedTime()
	u := deletingWidget(
		updateEntry("widget-ctl", at, finalizerFields(ownedFinalizer)),
		updateEntry("kubectl", at.Add(time.Hour), nil))

	trimmed, err := kube.TrimToLatestManager(u)
	require.NoError(t, err)

	cached := trimmed.(*unstructured.Unstructured)
	for _, entry := range cached.GetManagedFields() {
		assert.Nil(t, entry.FieldsV1, "field sets are dropped")
	}
	attrs := widgetAttributes(t, cached)
	assert.Equal(t, ownedFinalizer+"=widget-ctl",
		attrs[kube.AttrFinalizerManagers].AsText())
}

// An object that is not being deleted, or whose finalizer nobody owns in
// the managed fields, gets no owners.
func TestFinalizerManagersAreAbsentWithoutProof(t *testing.T) {
	live := deletingWidget(updateEntry("widget-ctl", fixedTime(),
		finalizerFields(ownedFinalizer)))
	live.SetDeletionTimestamp(nil)
	unowned := deletingWidget(updateEntry("kubectl", fixedTime(), nil))

	for name, u := range map[string]*unstructured.Unstructured{
		"not deleting": live, "unowned": unowned} {
		t.Run(name, func(t *testing.T) {
			trimmed, err := kube.TrimToLatestManager(u)
			require.NoError(t, err)

			attrs := widgetAttributes(t, trimmed.(*unstructured.Unstructured))

			_, found := attrs[kube.AttrFinalizerManagers]
			assert.False(t, found)
		})
	}
}

// A cluster object cannot plant owners: the transform replaces whatever
// the stamp held.
func TestFinalizerManagersIgnoreAPlantedStamp(t *testing.T) {
	u := deletingWidget(updateEntry("kubectl", fixedTime(), nil))
	u.SetAnnotations(map[string]string{
		"kwatch.abahmed.dev/finalizer-managers": ownedFinalizer + "=evil",
		"keep":                                  "me"})

	trimmed, err := kube.TrimToLatestManager(u)
	require.NoError(t, err)

	cached := trimmed.(*unstructured.Unstructured)
	assert.Equal(t, map[string]string{"keep": "me"}, cached.GetAnnotations())
	_, found := widgetAttributes(t, cached)[kube.AttrFinalizerManagers]
	assert.False(t, found)
}

func TestFinalizerManagersReadsTheAttribute(t *testing.T) {
	m := inventory.NewModel(inventory.Options{})
	id := inventory.CoreID(kube.KindSecret, "ops", "s")
	_, err := m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: fixedTime(), Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrFinalizerManagers: inventory.Text(
				"a.example.com/x=ctl-a\na.example.com/x=ctl-b\nb/y=ctl-c")}})
	require.NoError(t, err)
	e, _ := m.Entity(id)

	got := kube.FinalizerManagers(e)

	assert.Equal(t, map[string][]string{
		"a.example.com/x": {"ctl-a", "ctl-b"}, "b/y": {"ctl-c"}}, got)
}
