package detectors

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// stsCase is a StatefulSet watched by one registry, so the detector
// remembers when the set first fell short, as the engine does.
type stsCase struct {
	m        *inventory.Model
	id       inventory.EntityID
	registry *detection.Registry
}

// newStsCase is a set with three ready pods for an hour whose counts
// become desired and ready at t0; it is evaluated once at t0.
func newStsCase(desired, ready float64) *stsCase {
	c := &stsCase{m: newTestModel(),
		id:       newID(kube.KindStatefulSet, "shop", "queue"),
		registry: detection.NewRegistry(nil, NewWorkload(0))}
	c.counts(t0.Add(-time.Hour), 3, 3)
	for i := range 3 {
		c.pod(i, t0.Add(-time.Hour), true)
	}
	c.counts(t0, desired, ready)
	c.registry.Evaluate(c.m, t0, c.id)
	return c
}

func (c *stsCase) counts(at time.Time, desired, ready float64) {
	put(c.m, c.id, at, map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(desired),
		kube.AttrReadyReplicas: inventory.Number(ready),
	})
}

// pod puts pod i of the set, created at created.
func (c *stsCase) pod(i int, created time.Time, ready bool) {
	pod := newID(kube.KindPod, c.id.Namespace,
		c.id.Name+"-"+strconv.Itoa(i))
	put(c.m, pod, created, map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(created),
		kube.AttrReady:   inventory.Bool(ready),
		kube.AttrPhase:   inventory.Text("Running"),
	})
	link(c.m, pod, inventory.OwnedBy, c.id)
}

func (c *stsCase) findings(now time.Time) []string {
	return findingReasons(c.registry.Evaluate(c.m, now, c.id).Findings)
}

// An ordered scale-up that keeps moving is not unavailability: eight
// minutes in, with a pod ready every two minutes, nothing is reported.
func TestStatefulSetGrowingIsHeld(t *testing.T) {
	c := newStsCase(9, 3)
	for i := 3; i < 6; i++ {
		c.pod(i, t0.Add(time.Duration(i-3)*2*time.Minute), true)
	}
	c.counts(t0.Add(6*time.Minute), 9, 6)
	c.pod(6, t0.Add(6*time.Minute), false)
	assert.Empty(t, c.findings(t0.Add(8*time.Minute)))
}

// The hold ends when nothing has moved for DefaultRolloutStuck.
func TestStatefulSetGrowthThatStopsIsReported(t *testing.T) {
	c := newStsCase(9, 3)
	c.pod(3, t0, true)
	c.counts(t0.Add(2*time.Minute), 9, 4)
	c.pod(4, t0.Add(2*time.Minute), false)
	stuck := t0.Add(2*time.Minute + DefaultRolloutStuck)
	assert.Empty(t, c.findings(stuck.Add(-time.Minute)))
	assert.Contains(t, c.findings(stuck), reasons.StsUnavailable)
}

// A pod from before the scale-up that is not ready is an outage, not
// growth, however recently a new pod was added.
func TestStatefulSetGrowingWithOldPodDownIsReported(t *testing.T) {
	c := newStsCase(9, 3)
	c.counts(t0.Add(time.Minute), 9, 2)
	c.pod(1, t0.Add(-time.Hour), false)
	c.pod(3, t0.Add(5*time.Minute), false)
	assert.Contains(t, c.findings(t0.Add(6*time.Minute)),
		reasons.StsUnavailable)
}

// A set that lost a replica long after its size was set is not growing:
// its replacement pod is new, but the shortfall did not begin with a
// request for more.
func TestStatefulSetThatLostAReplicaIsNotGrowing(t *testing.T) {
	c := newStsCase(3, 2)
	c.pod(2, t0, false)
	assert.Contains(t, c.findings(t0.Add(6*time.Minute)),
		reasons.StsUnavailable)
}

// A set short of its size with no pod added since is not held: the
// usual wait applies.
func TestStatefulSetShortWithoutNewPodIsNotHeld(t *testing.T) {
	c := newStsCase(9, 3)
	assert.Contains(t, c.findings(t0.Add(6*time.Minute)),
		reasons.StsUnavailable)
}
