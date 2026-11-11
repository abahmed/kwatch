package release

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The previous rollout missed its deadline; the fix release is looked at
// before the controller has observed the new generation, so the
// Progressing reason is still the old rollout's. It must not fail the
// fix release.
func TestStaleDeadlineOfThePreviousRolloutIsIgnored(t *testing.T) {
	w := newWorld(kube.KindDeployment, "api")
	w.statusWith(2, 2, 1, deadlineReason, map[string]inventory.Value{
		kube.AttrGeneration:  inventory.Number(3),
		kube.AttrObservedGen: inventory.Number(2),
	})
	w.change(t0, "", image("api:2", "api:3"))
	var b Book
	w.look(&b, t0.Add(5*time.Second))
	require.Len(t, b.Records, 1)
	assert.Equal(t, Rolling, b.Records[0].Verdict)
	assert.False(t, b.Records[0].Stalled)
	assert.False(t, b.Records[0].Deadline)

	// The controller caught up and the rollout finished.
	w.statusWith(2, 2, 2, "NewReplicaSetAvailable",
		map[string]inventory.Value{
			kube.AttrGeneration:  inventory.Number(3),
			kube.AttrObservedGen: inventory.Number(3),
		})
	w.look(&b, t0.Add(time.Minute))
	w.look(&b, t0.Add(2*time.Minute+WatchWindow))
	assert.Equal(t, Healthy, b.Records[0].Verdict)
	assert.True(t, b.Records[0].Final)
}

// A deadline the controller reports for the current generation counts.
func TestDeadlineOfTheObservedGenerationCounts(t *testing.T) {
	w := newWorld(kube.KindDeployment, "api")
	w.statusWith(2, 2, 1, deadlineReason, map[string]inventory.Value{
		kube.AttrGeneration:  inventory.Number(3),
		kube.AttrObservedGen: inventory.Number(3),
	})
	w.change(t0, "", image("api:2", "api:3"))
	var b Book
	w.look(&b, t0.Add(11*time.Minute))
	assert.Equal(t, Failed, b.Records[0].Verdict)
	assert.True(t, b.Records[0].Deadline)
}

// pod adds a pod of the workload with one container that restarted
// restarts times. The model first sees it at seen; created, when set,
// is the pod's creation time.
func (w *world) pod(
	name string, seen, created time.Time, restarts float64,
) {
	pod := inventory.CoreID(kube.KindPod, "shop", name)
	container := kube.ContainerID("shop", name, "app")
	attrs := map[string]inventory.Value{}
	if !created.IsZero() {
		attrs[kube.AttrCreated] = inventory.Time(created)
	}
	w.m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: seen, Entity: pod, Attributes: attrs})
	w.m.Apply(inventory.Observation{Kind: inventory.Related,
		Source: "test", At: seen, Entity: pod, Relation: inventory.OwnedBy,
		Targets: []inventory.EntityID{w.id}})
	w.m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: seen, Entity: container,
		Attributes: map[string]inventory.Value{
			kube.AttrRestarts: inventory.Number(restarts)}})
	w.m.Apply(inventory.Observation{Kind: inventory.Related,
		Source: "test", At: seen, Entity: container,
		Relation: inventory.OwnedBy, Targets: []inventory.EntityID{pod}})
}

// After a kwatch restart every pod is first seen at the restart. A pod
// created long before the release is not the release's: its lifetime
// restarts must not count as restarts after the release.
func TestDetailLeavesOutPodsCreatedBeforeTheRelease(t *testing.T) {
	w := newWorld(kube.KindDeployment, "api")
	w.status(2, 1, 1, "")
	w.change(t0, "", image("api:1", "api:2"))
	restart := t0.Add(5 * time.Minute)
	w.pod("api-1-old", restart, t0.Add(-72*time.Hour), 40)
	w.pod("api-2-new", restart, t0.Add(10*time.Second), 3)
	var b Book
	Observe(&b, View{Model: w.m, Detail: true}, t0.Add(30*time.Minute))
	d := b.Records[0].Detail
	require.NotNil(t, d)
	require.NotNil(t, d.RestartsAfter)
	assert.InDelta(t, 6, *d.RestartsAfter, 0.1)
}

// Only old pods are left (the rollout made none yet): there is no
// "after" figure at all.
func TestDetailHasNoAfterRateWithoutNewPods(t *testing.T) {
	w := newWorld(kube.KindDeployment, "api")
	w.status(2, 2, 0, "")
	w.change(t0, "", image("api:1", "api:2"))
	w.pod("api-1-old", t0.Add(5*time.Minute), t0.Add(-72*time.Hour), 40)
	var b Book
	Observe(&b, View{Model: w.m, Detail: true}, t0.Add(30*time.Minute))
	assert.False(t, b.Records[0].Final)
	assert.Nil(t, b.Records[0].Detail)
}

// Two template changes within one second are two releases, and looking
// again does not add them twice.
func TestTwoReleasesInOneSecondAreKeptApart(t *testing.T) {
	w := newWorld(kube.KindDeployment, "api")
	w.status(2, 2, 2, "")
	w.change(t0.Add(100*time.Millisecond), "", image("api:1", "api:2"))
	w.change(t0.Add(600*time.Millisecond), "", image("api:2", "api:3"))
	var b Book
	w.look(&b, t0.Add(time.Minute))
	require.Len(t, b.Records, 2)
	// Newest first.
	assert.Equal(t, "deployment/shop/api@"+unix(t0)+"-2", b.Records[0].ID)
	assert.Equal(t, "api:3", b.Records[0].Images[0].After)
	assert.Equal(t, "deployment/shop/api@"+unix(t0), b.Records[1].ID)
	assert.Equal(t, Replaced, b.Records[1].Verdict)
	w.look(&b, t0.Add(2*time.Minute))
	assert.Len(t, b.Records, 2)
}

// A book saved before the suffix existed keeps its record: the first
// release of a second still has the plain name.
func TestSavedReleaseKeepsItsName(t *testing.T) {
	w := newWorld(kube.KindDeployment, "api")
	w.status(2, 2, 2, "")
	w.change(t0, "", image("api:1", "api:2"))
	b := Book{Records: []Record{{
		ID: "deployment/shop/api@" + unix(t0), Kind: "deployment",
		Namespace: "shop", Name: "api", At: t0, Verdict: Healthy,
		Final: true,
	}}}
	w.look(&b, t0.Add(time.Minute))
	assert.Len(t, b.Records, 1)
}

func unix(at time.Time) string {
	return strconv.FormatInt(at.Unix(), 10)
}
