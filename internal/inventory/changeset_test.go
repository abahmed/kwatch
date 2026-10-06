package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChangeSetsGroupRelease(t *testing.T) {
	h := newHistoryModel(t)
	h.change(t, h.config, 0, Change{Actor: "argocd-controller",
		Fields: []FieldChange{{Path: "data.url", After: "changed"}}})
	h.change(t, h.deployment, 30*time.Second,
		imageChange("registry/api:v2.2", "registry/api:v2.3"))

	sets := h.ChangeSets(h.pod, time.Time{})
	require.Len(t, sets, 1)
	set := sets[0]
	assert.Len(t, set.Changes, 2)
	assert.Equal(t, testTime, set.Start)
	assert.Equal(t, testTime.Add(30*time.Second), set.End)
	assert.Equal(t,
		"14:02 release by argocd-controller: image v2.3, "+
			"ConfigMap app-config", set.Label)
	assert.Regexp(t, `^cs-[0-9a-f]{10}$`, set.ID)
}

func TestChangeSetsPreferGitOpsApp(t *testing.T) {
	h := newHistoryModel(t)
	other := CoreID("configmap", "billing", "rates")
	h.apply(t, Observation{Kind: Observed, At: testTime, Entity: other})
	h.change(t, h.deployment, 0, Change{App: "argocd/shop",
		Fields: []FieldChange{{Path: "spec.replicas",
			Before: "2", After: "5"}}})
	h.change(t, other, time.Minute, Change{App: "argocd/shop",
		Fields: []FieldChange{{Path: "data.rate", After: "changed"}}})

	sets := h.RecentChangeSets(time.Time{})
	require.Len(t, sets, 1)
	assert.Equal(t, "argocd/shop", sets[0].Who)
	assert.Equal(t,
		"14:02 release by argocd/shop: ConfigMap rates, api scale 2→5",
		sets[0].Label)
}

func TestChangeSetsSplitByTimeAndRelation(t *testing.T) {
	h := newHistoryModel(t)
	unrelated := CoreID("deployment", "billing", "worker")
	h.apply(t, Observation{Kind: Observed, At: testTime, Entity: unrelated})
	h.change(t, h.deployment, 0, imageChange("api:v1", "api:v2"))
	h.change(t, unrelated, 10*time.Second, Change{Actor: "helm",
		Fields: []FieldChange{{Path: "spec.template"}}})
	h.change(t, h.config, 5*time.Minute, Change{Actor: "kubectl",
		Fields: []FieldChange{{Path: "data.url", After: "changed"}}})

	assert.Len(t, h.RecentChangeSets(time.Time{}), 3)
	sets := h.ChangeSets(h.deployment, time.Time{})
	require.Len(t, sets, 2)
	assert.Equal(t, h.deployment, sets[0].Changes[0].Entity)
	assert.Equal(t, h.config, sets[1].Changes[0].Entity)
	assert.Len(t, h.ChangeSets(h.deployment, testTime.Add(time.Minute)), 1)
}

func TestChangeSetIDIsStableWhenChangesJoin(t *testing.T) {
	h := newHistoryModel(t)
	h.change(t, h.deployment, 0, imageChange("api:v1", "api:v2"))
	before := h.ChangeSets(h.deployment, time.Time{})[0].ID
	h.change(t, h.config, time.Minute, Change{
		Fields: []FieldChange{{Path: "data.url", After: "changed"}}})
	after := h.ChangeSets(h.deployment, time.Time{})
	require.Len(t, after, 1)
	assert.Equal(t, before, after[0].ID)
	assert.Len(t, after[0].Changes, 2)
}

func TestChangeSetsSamePersonSameNamespace(t *testing.T) {
	h := newHistoryModel(t)
	other := CoreID("deployment", "shop", "web")
	h.apply(t, Observation{Kind: Observed, At: testTime, Entity: other})
	h.change(t, h.deployment, 0, Change{Actor: "kubectl-edit",
		Fields: []FieldChange{{Path: "spec.template"}}})
	h.change(t, other, time.Minute, Change{Actor: "kubectl-edit",
		Fields: []FieldChange{{Path: "spec.template"}}})
	h.change(t, other, 90*time.Second, Change{
		Actor:  "kube-controller-manager",
		Fields: []FieldChange{{Path: "spec.replicas"}}})
	assert.Len(t, h.RecentChangeSets(time.Time{}), 1)
}

func TestImageTag(t *testing.T) {
	tests := map[string]string{
		"registry:5000/team/api:v2.3": "v2.3",
		"api":                         "api",
		"registry:5000/api":           "registry:5000/api",
		"api@sha256:0123456789abcdef": "@0123456789ab",
	}
	for image, want := range tests {
		assert.Equal(t, want, imageTag(image), image)
	}
}

func TestChangeSetsStopGrowingAfterTheMaxSpan(t *testing.T) {
	h := newHistoryModel(t)
	// A controller that never stops: each change is within the window of
	// the one before, for 40 minutes.
	for i := range 27 {
		h.change(t, h.deployment, time.Duration(i)*90*time.Second,
			Change{App: "argocd/shop", Fields: []FieldChange{{
				Path: "spec.replicas", After: "3"}}})
	}
	sets := h.RecentChangeSets(time.Time{})
	require.Greater(t, len(sets), 1, "one chain is cut into several sets")
	total := 0
	for _, set := range sets {
		assert.LessOrEqual(t, set.End.Sub(set.Start), ChangeSetMaxSpan)
		total += len(set.Changes)
	}
	assert.Equal(t, 27, total)
}
