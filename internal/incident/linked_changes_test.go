package incident

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestLinkedChangesOutliveTheChangeHistory(t *testing.T) {
	r := newRig(t, Config{})
	web := entity(kube.KindPod, "web")
	deploy := entity(kube.KindDeployment, "web")
	r.observe(deploy)
	r.observe(web)
	r.relate(web, inventory.OwnedBy, deploy)
	r.change(deploy, -5*time.Minute, image("web:1", "web:2"))
	r.raise(at(0), sig(web, "CrashLoopBackOff", 2))

	got := r.only().Journal.Changes
	require.Len(t, got, 1)
	assert.Equal(t, deploy, got[0].Entity)
	assert.Equal(t, "web:2", got[0].Fields[0].After)

	records := r.m.Export()
	require.Len(t, records[0].LinkedChanges, 1)
}

func TestLinkedChangesAreLookedForOncePerMinute(t *testing.T) {
	r := newRig(t, Config{})
	web := entity(kube.KindPod, "web")
	deploy := entity(kube.KindDeployment, "web")
	r.observe(deploy)
	r.observe(web)
	r.relate(web, inventory.OwnedBy, deploy)
	r.raise(at(0), sig(web, "CrashLoopBackOff", 2))
	r.change(deploy, 10*time.Second, image("web:2", "web:3"))
	r.tick(at(30 * time.Second))
	assert.Empty(t, r.only().Journal.Changes, "looked at the open")
	r.tick(at(time.Minute))
	assert.Len(t, r.only().Journal.Changes, 1)
}

func TestMergeLinkedKeepsWhatPrecededTheFailure(t *testing.T) {
	onset := at(0)
	var fresh []inventory.Change
	for i := 0; i < maxLinkedChanges+4; i++ {
		fresh = append(fresh, inventory.Change{
			Entity: entity(kube.KindConfigMap, "c"+string(rune('a'+i))),
			At:     onset.Add(time.Duration(i-2) * time.Minute)})
	}
	got := mergeLinked(nil, fresh, onset)
	require.Len(t, got, maxLinkedChanges)
	assert.Equal(t, onset, got[0].At, "before onset first, newest first")
	assert.Equal(t, onset.Add(-2*time.Minute), got[2].At)
	assert.Equal(t, onset.Add(13*time.Minute), got[3].At,
		"then during, newest first; the oldest during are cut")

	again := mergeLinked(got, fresh[:3], onset)
	assert.Equal(t, got, again, "a held change is not added twice")
}

func TestSafeChangeNamesSecretKeysOnly(t *testing.T) {
	secret := inventory.Change{
		Entity: entity(kube.KindSecret, "db"),
		Fields: []inventory.FieldChange{
			{Path: "data.password", Before: "hunter2", After: "s3cret"},
			{Path: "data.user", After: "added"},
			{Path: "data.host"},
			{Path: "data.port", Before: "5432"},
			{Path: "data.mode", After: "rw"},
		},
	}
	got := SafeChange(secret)
	assert.Equal(t, []inventory.FieldChange{
		{Path: "data.password", After: "changed"},
		{Path: "data.user", After: "added"},
		{Path: "data.host", After: "changed"},
		{Path: "data.port", After: "removed"},
		{Path: "data.mode", After: "added"},
	}, got.Fields, "a data field always says how it was edited")

	deploy := inventory.Change{
		Entity: entity(kube.KindDeployment, "api"),
		Actor:  strings.Repeat("a", 400),
		Fields: []inventory.FieldChange{
			{Path: "stringData.token", Before: "x", After: "y"},
			{Path: "containers[api].image", Before: "api:1",
				After: "api:2"},
		},
	}
	got = SafeChange(deploy)
	assert.Equal(t, inventory.FieldChange{Path: "stringData.token",
		After: "changed"}, got.Fields[0])
	assert.Equal(t, "api:2", got.Fields[1].After)
	assert.Len(t, got.Actor, maxChangeValue)
}

func TestSafeChangeBoundsFields(t *testing.T) {
	change := inventory.Change{Entity: entity(kube.KindDeployment, "api")}
	for i := 0; i < maxChangeFields+3; i++ {
		change.Fields = append(change.Fields, inventory.FieldChange{
			Path: "spec.f", After: strings.Repeat("v", 500)})
	}
	got := SafeChange(change)
	require.Len(t, got.Fields, maxChangeFields)
	assert.Len(t, got.Fields[0].After, maxChangeValue)
	assert.Len(t, change.Fields, maxChangeFields+3, "input untouched")
}

// Without change correlation (Community) the journal links no changes
// and takes no snapshots; the lifecycle is the same.
func TestNoJournalDetailLinksNoChanges(t *testing.T) {
	run := func(cfg Config) Incident {
		r := newRig(t, cfg)
		web := entity(kube.KindPod, "web")
		deploy := entity(kube.KindDeployment, "web")
		r.observe(deploy)
		r.observe(web)
		r.relate(web, inventory.OwnedBy, deploy)
		r.change(deploy, -5*time.Minute, image("web:1", "web:2"))
		r.raise(at(0), sig(web, "CrashLoopBackOff", 2))
		r.tick(at(2 * time.Minute))
		return r.only()
	}
	full := run(Config{})
	plain := run(Config{NoJournalDetail: true})

	assert.NotEmpty(t, full.Journal.Changes)
	assert.Empty(t, plain.Journal.Changes)
	assert.Empty(t, plain.Journal.Breaks)
	assert.Equal(t, full.State, plain.State)
	assert.Equal(t, full.Tier, plain.Tier)
	assert.Equal(t, full.Journal.Transitions, plain.Journal.Transitions)
	assert.Equal(t, full.Journal.ErrorKey, plain.Journal.ErrorKey)
}
