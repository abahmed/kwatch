package incident

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// A failure left without a cause while a candidate outside its own
// workload was considered and dropped has an unclear cause.
func TestManagerMarksCauseUnclearAfterOutsideRejection(t *testing.T) {
	r := newRig(t, Config{})
	pod, _ := workload(r, "web")
	r.rule.rejected[pod] = []explain.Rejection{{
		Root:   entity(kube.KindSecret, "db"),
		Reason: "confidence below the floor"}}

	r.raise(at(0), podSig("web-0"))

	p := r.only()
	assert.Nil(t, p.Cause)
	assert.True(t, p.CauseUnclear)
}

// Rejections inside the failing workload, its own pods or the workload
// itself, are not upstream candidates: a crash with nothing upstream
// has no unclear cause, it has none.
func TestManagerOwnWorkloadRejectionIsNotUnclear(t *testing.T) {
	r := newRig(t, Config{})
	pod, deploy := workload(r, "web")
	r.rule.rejected[pod] = []explain.Rejection{
		{Root: pod, Reason: "confidence below the floor"},
		{Root: deploy, Reason: "confidence below the floor"},
	}

	r.raise(at(0), podSig("web-0"))

	p := r.only()
	assert.Nil(t, p.Cause)
	assert.False(t, p.CauseUnclear)
}

// A found cause ends the doubt, and a later explanation without one
// does not revive it while the cause stands.
func TestManagerFoundCauseClearsUnclear(t *testing.T) {
	r := newRig(t, Config{})
	pod, _ := workload(r, "web")
	node := entity(kube.KindNode, "n1")
	r.rule.rejected[pod] = []explain.Rejection{{
		Root: node, Reason: "confidence below the floor"}}
	web := podSig("web-0")
	r.raise(at(0), web)
	require.True(t, r.only().CauseUnclear)

	delete(r.rule.rejected, pod)
	r.cause(pod, node, "node ran out of memory")
	r.raise(at(1), web)

	p := r.of(node)
	assert.NotNil(t, p.Cause)
	assert.False(t, p.CauseUnclear)
}

func TestRecordCauseUnclearRoundTrip(t *testing.T) {
	r := newRig(t, Config{})
	pod, _ := workload(r, "web")
	r.rule.rejected[pod] = []explain.Rejection{{
		Root: entity(kube.KindConfigMap, "cfg"), Reason: "vetoed"}}
	announced(t, r, podSig("web-0"))

	raw, err := json.Marshal(r.m.Export())
	require.NoError(t, err)
	var records []Record
	require.NoError(t, json.Unmarshal(raw, &records))
	fresh := newRig(t, Config{})
	fresh.m.Restore(records, at(0))
	assert.True(t, fresh.only().CauseUnclear)

	// Records written before the field existed restore without it.
	var legacy []Record
	require.NoError(t, json.Unmarshal(
		[]byte(`[{"ID":"inc-1","State":1}]`), &legacy))
	assert.False(t, restored(legacy[0]).CauseUnclear)
}

// The digest ignores the flag: learning that the cause is unclear is
// not news worth an update.
func TestManagerCauseUnclearDoesNotChangeDigest(t *testing.T) {
	r := newRig(t, Config{})
	pod, _ := workload(r, "web")
	web := podSig("web-0")
	announced(t, r, web)
	before := r.only()
	require.False(t, before.CauseUnclear)

	r.rule.rejected[pod] = []explain.Rejection{{
		Root:   inventory.CoreID(kube.KindSecret, "shop", "db"),
		Reason: "confidence below the floor"}}
	r.raise(at(DefaultSettle+1), web)

	after := r.only()
	assert.True(t, after.CauseUnclear)
	assert.Equal(t, before.Digest, after.Digest)
}
