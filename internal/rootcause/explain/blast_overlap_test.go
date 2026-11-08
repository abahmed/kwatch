package explain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// blastView: api has four pods, the first two crash-looping, the last
// two healthy, and its Deployment fails as a whole. The view's blast map
// is filled as rank fills it, from the effects each candidate covers.
func blastView(t *testing.T) (*view, []inventory.EntityID) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 4)
	for _, pod := range pods[:2] {
		f.fail(containerOf(pod), detection.ModeCrashLoop,
			detection.Failing, 2, "exit 1")
	}
	f.fail(inventory.CoreID(kube.KindDeployment, "shop", "api"),
		detection.ModeUnavailable, detection.Failing, 2, "")
	return newView(f.snapshot()), pods
}

// A candidate whose effect is the failing workload reaches its healthy
// pods too. They are not failing pods: the share never passes one and
// the reward never passes BlastOverlapWeight.
func TestBlastOverlapCountsOnlyFailingPods(t *testing.T) {
	v, pods := blastView(t)
	deployment := inventory.CoreID(kube.KindDeployment, "shop", "api")
	cfg := inventory.CoreID(kube.KindConfigMap, "shop", "cfg")
	other := inventory.CoreID(kube.KindConfigMap, "shop", "other")

	reached := v.failingPodsOf([]inventory.EntityID{deployment})
	require.Len(t, reached, 4, "the workload reaches every pod")
	failing := v.failingOnly(reached)
	require.Len(t, failing, 2)
	v.blast = map[inventory.EntityID]map[inventory.EntityID]bool{
		cfg: failing, other: {pods[0]: true}}

	out := scoreBlastOverlap(v, &candidate{id: cfg})

	require.Equal(t, rootcause.ProofBlastOverlap, out.code, out.text)
	assert.LessOrEqual(t, out.count, out.total, "a share of at most one")
	assert.LessOrEqual(t, out.weight, BlastOverlapWeight+1e-9)
	assert.Equal(t, 2, out.count)
}

// A cluster-scoped candidate (no namespace) is scored over the
// namespaces of its failing pods, like the namespaced one it competes
// with, not skipped.
func TestBlastOverlapScoresClusterScopedAndNamespacedAlike(t *testing.T) {
	v, pods := blastView(t)
	namespaced := inventory.CoreID(kube.KindConfigMap, "shop", "cfg")
	cluster := inventory.CoreID(kube.KindStorageClass, "", "fast")
	failing := map[inventory.EntityID]bool{pods[0]: true, pods[1]: true}
	v.blast = map[inventory.EntityID]map[inventory.EntityID]bool{
		namespaced: failing, cluster: failing}

	a := scoreBlastOverlap(v, &candidate{id: namespaced})
	b := scoreBlastOverlap(v, &candidate{id: cluster})

	assert.Equal(t, rootcause.ProofBlastOverlap, a.code)
	assert.Equal(t, rootcause.ProofBlastOverlap, b.code,
		"a cluster-scoped candidate is not skipped")
	assert.Equal(t, a.count, b.count)
	assert.Equal(t, a.total, b.total)
	assert.InDelta(t, a.weight, b.weight, 1e-9)
}
