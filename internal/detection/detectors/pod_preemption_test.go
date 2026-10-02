package detectors

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// preemptedPods adds pods of one ReplicaSet, the first victims of them
// preempted at the given ages, and returns the first pod.
func preemptedPods(
	model *inventory.Model, total int, ages ...time.Duration,
) inventory.EntityID {
	owner := inventory.EntityID{Kind: "ReplicaSet", Namespace: "default",
		Name: "web-5d9"}
	var first inventory.EntityID
	for i := 0; i < total; i++ {
		id := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: fmt.Sprintf("web-5d9-%d", i)}
		attrs := map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Running"),
		}
		if i < len(ages) {
			attrs[kube.AttrDeleting] = inventory.Bool(true)
			conditionAttrs(attrs, "DisruptionTarget", "True",
				"PreemptionByScheduler", podNodeNow.Add(-ages[i]))
		}
		observeEntity(model, id, podNodeNow, attrs)
		relateEntity(model, id, inventory.OwnedBy, owner)
		if i == 0 {
			first = id
		}
	}
	return first
}

func TestPodPreemptedRepeatedly(t *testing.T) {
	model := newTestModel()
	id := preemptedPods(model, 4, time.Minute, 5*time.Minute,
		14*time.Minute)
	found := classified(NewPod(PodThresholds{}).Detect(
		testDetectorContext(model, podNodeNow), entityOf(model, id)))
	require.Len(t, found, 1)
	assert.Equal(t, reasons.PodPreemptedRepeatedly, found[0].Reason)
	assert.Equal(t, "Preempted.Repeatedly", string(found[0].Mode))
	assert.Equal(t, detection.Degraded, found[0].Health)
	assert.Equal(t, "3", found[0].Evidence[0].Value)
}

func TestPodPreemptionBelowStorm(t *testing.T) {
	cases := map[string][]time.Duration{
		"two victims":          {time.Minute, 2 * time.Minute},
		"third outside window": {time.Minute, 2 * time.Minute, time.Hour},
	}
	for name, ages := range cases {
		model := newTestModel()
		id := preemptedPods(model, 4, ages...)
		found := NewPod(PodThresholds{}).Detect(
			testDetectorContext(model, podNodeNow), entityOf(model, id))
		assert.NotContains(t, findingReasons(found),
			reasons.PodPreemptedRepeatedly, name)
	}
}
