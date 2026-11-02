package incident

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A failing member pod is never cut for the pods its workload owns,
// even when its name sorts after all of them.
func TestPodsOfKeepsMemberPodsBeforeOwnedPods(t *testing.T) {
	r := newRig(t, Config{})
	deploy := entity(kube.KindDeployment, "api")
	r.observe(deploy)
	for i := range maxPods + 8 {
		r.relate(entity(kube.KindPod, fmt.Sprintf("api-%02d", i)),
			inventory.OwnedBy, deploy)
	}
	failing := entity(kube.KindPod, "api-zzz")
	r.relate(failing, inventory.OwnedBy, deploy)

	pods := PodsOf(r.model, deploy, []inventory.EntityID{failing})

	assert.Len(t, pods, maxPods)
	assert.Equal(t, failing, pods[0])
	assert.Equal(t, entity(kube.KindPod, "api-00"), pods[1])
	assert.Equal(t, pods, PodsOf(r.model, deploy,
		[]inventory.EntityID{failing}), "deterministic")
}

func TestFailingContainersNeedsMoreThanARestart(t *testing.T) {
	yes, no := true, false
	rows := []ContainerState{
		{Container: "healthy", Ready: &yes, Restarts: 5},
		{Container: "unknown", Restarts: 5},
		{Container: "unready", Ready: &no},
		{Container: "waiting", Ready: &yes, StateReason: "Waiting"},
		{Container: "fine", Ready: &yes},
	}

	var got []string
	for _, row := range failingContainers(rows) {
		got = append(got, row.Container)
	}

	assert.Equal(t, []string{"unknown", "unready", "waiting"}, got)
}
