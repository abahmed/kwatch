package incident

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestManagerExplainsWithTheConfiguredSight(t *testing.T) {
	sight := &kindSight{hidden: map[inventory.Kind]bool{
		kube.KindSecret: true,
	}}
	r := newRig(t, Config{Verifiable: sight.verifiable})
	web := podSig("web")
	r.rule.unverified[web.Entity] = []string{"secrets in billing"}
	r.raise(at(0), web)

	verifiable := r.rule.last.Verifiable
	if verifiable == nil || verifiable(kube.KindSecret) {
		t.Fatal("the explainer must see what kwatch cannot observe")
	}
	p := r.only()
	assert.Nil(t, p.Cause)
	assert.Equal(t, []string{"secrets in billing"}, p.Unverified)
}

func TestManagerKeepsUnverifiedNotes(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.rule.unverified[web.Entity] = []string{"config maps in shop"}
	announced(t, r, web)
	before := r.only()
	assert.Equal(t, []string{"config maps in shop"}, before.Unverified)

	// A later explanation without the note does not drop it.
	delete(r.rule.unverified, web.Entity)
	r.raise(at(DefaultSettle+1), web)
	assert.Equal(t, before.Unverified, r.only().Unverified)
	assert.Equal(t, before.Digest, r.only().Digest)
}

func TestRecordUnverifiedRoundTrip(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.rule.unverified[web.Entity] = []string{"secrets in billing"}
	announced(t, r, web)

	raw, err := json.Marshal(r.m.Export())
	require.NoError(t, err)
	var records []Record
	require.NoError(t, json.Unmarshal(raw, &records))
	fresh := newRig(t, Config{})
	fresh.m.Restore(records, at(0))
	assert.Equal(t, []string{"secrets in billing"},
		fresh.only().Unverified)

	// Records written before the field existed restore without it.
	var legacy []Record
	require.NoError(t, json.Unmarshal(
		[]byte(`[{"ID":"inc-1","State":1}]`), &legacy))
	assert.Nil(t, restored(legacy[0]).Unverified)
}
