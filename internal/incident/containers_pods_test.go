package incident

import (
	"fmt"
	"testing"
	"time"

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
	cutoff := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	rows := []ContainerState{
		{Container: "healthy", Ready: &yes, Restarts: 5},
		{Container: "unknown", Restarts: 5},
		{Container: "unready", Ready: &no},
		{Container: "waiting", Ready: &yes, StateReason: "Waiting"},
		{Container: "fine", Ready: &yes},
		{Container: "old-oom", Ready: &yes, Restarts: 1,
			LastReason: "OOMKilled", LastFinished: cutoff.Add(-time.Hour)},
		{Container: "old-untimed", Ready: &yes, LastReason: "Error"},
		{Container: "recent-oom", Ready: &yes, Restarts: 1,
			LastReason: "OOMKilled", LastFinished: cutoff.Add(time.Minute)},
	}

	var got []string
	for _, row := range failingContainers(rows, cutoff) {
		got = append(got, row.Container)
	}

	assert.Equal(t, []string{"unknown", "unready", "waiting",
		"recent-oom"}, got)
}

// A Ready container's last termination counts from the incident's
// onset or RecentWindow ago, whichever is earlier.
func TestBreakCutoffUsesEarlierOfOnsetAndWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	old := &Incident{Opened: now.Add(-2 * time.Hour)}
	fresh := &Incident{Opened: now.Add(-time.Minute)}

	assert.Equal(t, old.Opened, breakCutoff(old, now))
	assert.Equal(t, now.Add(-RecentWindow), breakCutoff(fresh, now))
	assert.Equal(t, now.Add(-RecentWindow), breakCutoff(&Incident{}, now))
}
