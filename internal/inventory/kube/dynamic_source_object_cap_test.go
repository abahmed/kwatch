package kube

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/abahmed/kwatch/internal/inventory"
)

func widgetID(name string) inventory.EntityID {
	return inventory.NewEntityID("example.com", "widget", "ns", name)
}

func TestDynamicSourceCapHidesOnlyTheRefusedObject(t *testing.T) {
	f := newDynamicFixtureWith(
		func(cfg *DynamicConfig) { cfg.MaxObjectsPerKind = 1 },
		widgetCRD(), widget("w"), widget("x"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)
	s := &Source{cfg: SourceConfig{Dynamic: f.src}}

	require.False(t, s.Verifiable("widget"))
	w, x := s.VerifiableObject(widgetID("w")),
		s.VerifiableObject(widgetID("x"))
	assert.True(t, w != x, "one object is admitted, the other refused")
	assert.True(t, s.VerifiableObject(widgetID("deleted-long-ago")),
		"an object the synced watch does not report does not exist")
}

func TestDynamicSourceLostAccessHidesEveryObjectOfACappedKind(t *testing.T) {
	f := newDynamicFixtureWith(
		func(cfg *DynamicConfig) { cfg.MaxObjectsPerKind = 1 },
		widgetCRD(), widget("w"), widget("x"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)
	s := &Source{cfg: SourceConfig{Dynamic: f.src}}

	f.refuseWidgets(t, forbidden("widgets"))

	assert.Equal(t, ReasonPermissionDenied, f.widgetState(t).Reason)
	assert.False(t, s.VerifiableObject(widgetID("w")))
	assert.False(t, s.VerifiableObject(widgetID("x")))
}

func TestDynamicSourceCapDoesNotHideAnotherReason(t *testing.T) {
	f := newDynamicFixtureWith(
		func(cfg *DynamicConfig) { cfg.MaxObjectsPerKind = 1 },
		widgetCRD(), widget("w"), widget("x"))
	ctx, cancel := context.WithCancel(context.Background())
	defer f.stop(cancel)
	f.src.reconcile(ctx)
	f.waitSynced(t, ctx)

	f.src.mu.Lock()
	f.src.left[widgetGVR] = kindIssue{"widget", ReasonWatchBudget}
	f.src.mu.Unlock()

	assert.Equal(t, ReasonWatchBudget, f.widgetState(t).Reason)
}

func TestObjectRefusedKeysClusterScopedObjectsByName(t *testing.T) {
	a := newAdmission(widgetGVR, 0, &objectBudget{limit: 1})
	a.left["labels"] = true
	d := &DynamicSource{running: map[schema.GroupVersionResource]*dynamicWatch{
		widgetGVR: {kind: "clusterpolicy", admission: a},
	}}

	assert.True(t, d.ObjectRefused(inventory.NewEntityID(
		"kyverno.io", "clusterpolicy", "", "labels")))
	assert.False(t, d.ObjectRefused(inventory.NewEntityID(
		"kyverno.io", "clusterpolicy", "", "other")))
}

func TestSourceVerifiableObjectJudgesOnlyCappedKindsByObject(t *testing.T) {
	s := &Source{cfg: SourceConfig{Dynamic: fakeDynamicKinds{
		"capped": {Reason: ReasonObjectCap},
		"denied": {Reason: ReasonPermissionDenied},
		"fine":   {Synced: true},
	}}}

	for _, tc := range []struct {
		kind inventory.Kind
		name string
		want bool
	}{
		{"capped", "admitted", true},
		{"capped", "refused", false},
		{"denied", "admitted", false},
		{"fine", "refused", true},
		{"unwatched", "any", true},
	} {
		got := s.VerifiableObject(inventory.CoreID(tc.kind, "", tc.name))
		assert.Equal(t, tc.want, got, "%s/%s", tc.kind, tc.name)
	}
}
