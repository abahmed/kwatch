package inventory

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configEdit() Change {
	return Change{Fields: []FieldChange{{Path: "data.url", After: "changed"}}}
}

func TestChangeSetIDSurvivesFirstChangeEviction(t *testing.T) {
	h := newHistoryModel(t)
	h.change(t, h.deployment, 0, imageChange("api:v1", "api:v2"))
	h.change(t, h.config, time.Minute, configEdit())
	id := h.RecentChangeSets(time.Time{})[0].ID

	h.Prune(testTime.Add(30 * time.Second))

	sets := h.RecentChangeSets(time.Time{})
	require.Len(t, sets, 1)
	assert.Equal(t, id, sets[0].ID)
	require.Len(t, sets[0].Changes, 1)
	assert.Equal(t, h.config, sets[0].Changes[0].Entity)
	assert.Equal(t, testTime.Add(time.Minute), sets[0].Start)
}

func TestChangeSetMergeKeepsOldestID(t *testing.T) {
	h := newHistoryModel(t)
	web := CoreID("deployment", "shop", "web")
	h.apply(t, Observation{Kind: Observed, At: testTime, Entity: web})
	h.change(t, h.deployment, 0, imageChange("api:v1", "api:v2"))
	h.change(t, web, 90*time.Second, Change{App: "argocd/shop",
		Fields: []FieldChange{{Path: "spec.template"}}})
	before := h.RecentChangeSets(time.Time{})
	require.Len(t, before, 2)

	// The ConfigMap edit is used by api and shares web's application, so
	// it bridges both sets.
	bridge := configEdit()
	bridge.App = "argocd/shop"
	h.change(t, h.config, time.Minute, bridge)

	after := h.RecentChangeSets(time.Time{})
	require.Len(t, after, 1)
	assert.Equal(t, before[0].ID, after[0].ID)
	assert.Len(t, after[0].Changes, 3)
}

func TestChangeSetMergeResolvesTheAbsorbedID(t *testing.T) {
	h := newHistoryModel(t)
	web := CoreID("deployment", "shop", "web")
	h.apply(t, Observation{Kind: Observed, At: testTime, Entity: web})
	h.change(t, h.deployment, 0, imageChange("api:v1", "api:v2"))
	h.change(t, web, 90*time.Second, Change{App: "argocd/shop",
		Fields: []FieldChange{{Path: "spec.template"}}})
	before := h.RecentChangeSets(time.Time{})
	require.Len(t, before, 2)
	bridge := configEdit()
	bridge.App = "argocd/shop"
	h.change(t, h.config, time.Minute, bridge)

	survivor, ok := h.ChangeSet(before[1].ID)
	require.True(t, ok, "an absorbed ID still finds its set")
	assert.Equal(t, before[0].ID, survivor.ID)
	assert.Equal(t, []string{before[1].ID}, survivor.Aliases)
	_, ok = h.ChangeSet("cs-unknown")
	assert.False(t, ok)
	_, ok = h.ChangeSet("")
	assert.False(t, ok)
}

func TestChangeSetDoesNotSplitWhenEdgesGo(t *testing.T) {
	h := newHistoryModel(t)
	h.change(t, h.deployment, 0, imageChange("api:v1", "api:v2"))
	h.change(t, h.pod, 30*time.Second, Change{
		Fields: []FieldChange{{Path: "spec.activeDeadlineSeconds"}}})
	id := h.RecentChangeSets(time.Time{})[0].ID

	// Deleting the pod drops its owner edge; the set built while the
	// edge existed stays one set.
	h.apply(t, Observation{
		Kind: Gone, At: testTime.Add(40 * time.Second), Entity: h.pod,
	})

	sets := h.RecentChangeSets(time.Time{})
	require.Len(t, sets, 1)
	assert.Equal(t, id, sets[0].ID)
	assert.Len(t, sets[0].Changes, 3)
}

func TestChangeSetChurnDoesNotEvictWorkloadEdits(t *testing.T) {
	h := newHistoryModel(t)
	h.change(t, h.deployment, 0, imageChange("api:v1", "api:v2"))
	for i := 0; i < 2*DefaultMaxRecentChanges; i++ {
		pod := CoreID("pod", "batch", "drained-"+strconv.Itoa(i))
		h.change(t, pod, time.Duration(i)*time.Millisecond,
			Change{Created: true})
	}

	sets := h.RecentChangeSets(time.Time{})
	members := 0
	found := false
	for _, set := range sets {
		members += len(set.Changes)
		for _, change := range set.Changes {
			found = found || change.Entity == h.deployment
		}
	}
	assert.True(t, found, "the Deployment edit must survive the churn")
	assert.Equal(t, 1+DefaultMaxChurnChanges, members,
		"churn is bounded by its own ring")
}

func TestChangeSetMintIDAvoidsClash(t *testing.T) {
	c := newSetCache()
	first := Change{Entity: CoreID("deployment", "shop", "api"), At: testTime}
	id := c.mintID(first)
	c.sets = append(c.sets, &liveSet{set: ChangeSet{ID: id}})

	assert.Equal(t, id+"-2", c.mintID(first))
}
