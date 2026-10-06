package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An autoscaler incident whose own finding cleared does not resolve
// while the workload it scales is still below its replicas with
// crash-looping pods; it resolves once the workload is healthy.
func TestIncidentDoesNotResolveWhileItsWorkloadIsBroken(t *testing.T) {
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, 0, 5)
	hpa := entity(kube.KindHPA, "api")
	r.relate(hpa, inventory.Scales, deploy)
	maxed := sig(hpa, reasons.HPAMaxedOut, detection.Warning)
	r.raise(at(0), maxed)
	require.Len(t, r.tick(at(DefaultSettle)), 1)

	r.clear(at(5*time.Minute), maxed)
	r.tick(at(5 * time.Minute))
	wantNone(t, r.tick(at(5*time.Minute+DefaultHold+time.Minute)))
	wantNone(t, r.tick(at(2*time.Hour)))
	assert.Equal(t, Recovering, r.only().State)

	r.setAttrs(deploy, map[string]inventory.Value{
		kube.AttrReadyReplicas: inventory.Number(2)})
	r.setAttrs(pod, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(true)})
	ds := r.tick(at(2*time.Hour + time.Minute))
	require.Len(t, ds, 1)
	assert.Equal(t, Resolve, ds[0].Action)
}

// A workload that is short but whose pods never failed (a slow
// rollout) does not hold a resolve back.
func TestShortWorkloadWithoutFailingPodsDoesNotBlockResolve(t *testing.T) {
	r := newRig(t, Config{})
	deploy, pod := r.workloadRig(t, 2, 1, 0)
	hpa := entity(kube.KindHPA, "api")
	r.relate(hpa, inventory.Scales, deploy)
	_ = pod
	maxed := sig(hpa, reasons.HPAMaxedOut, detection.Warning)
	r.raise(at(0), maxed)
	require.Len(t, r.tick(at(DefaultSettle)), 1)
	r.clear(at(5*time.Minute), maxed)
	r.tick(at(5 * time.Minute))
	ds := r.tick(at(5*time.Minute + DefaultHold))
	require.Len(t, ds, 1)
	assert.Equal(t, Resolve, ds[0].Action)
}

// A flapping incident is held the same way.
func TestFlappingIncidentDoesNotResolveWhileWorkloadIsBroken(t *testing.T) {
	r := newRig(t, Config{})
	deploy, _ := r.workloadRig(t, 2, 0, 5)
	web := podSig("web")
	container := entity(kube.KindContainer, "web.app")
	r.setAttrs(web.Entity, map[string]inventory.Value{
		kube.AttrReady: inventory.Bool(false)})
	r.setAttrs(container, map[string]inventory.Value{
		kube.AttrRestarts: inventory.Number(9)})
	r.relate(web.Entity, inventory.OwnedBy, deploy)
	r.relate(container, inventory.PartOf, web.Entity)
	runCycles(r, 2*time.Minute, 4*time.Minute, 30*time.Minute)
	require.Equal(t, Flapping, r.only().State)

	last := 30 * time.Minute
	r.clear(at(last), web)
	r.tick(at(last))
	wantNone(t, r.tick(at(last+DefaultMaxHold)))
	wantNone(t, r.tick(at(last+2*DefaultMaxHold)))
	assert.Equal(t, Flapping, r.only().State)
}

// The extra hold is bounded: a workload that stays short with a failing
// pod no detector flags must not keep an incident (and its paging alert)
// open forever. The incident resolves StillBrokenMax after it recovered,
// and the manager arms a timer for that moment instead of waiting for the
// heartbeat. A workload that really is down is handed back by the
// coverage check.
func TestStillBrokenHoldIsBounded(t *testing.T) {
	r := newRig(t, Config{})
	deploy, _ := r.workloadRig(t, 2, 0, 5)
	hpa := entity(kube.KindHPA, "api")
	r.relate(hpa, inventory.Scales, deploy)
	maxed := sig(hpa, reasons.HPAMaxedOut, detection.Warning)
	r.raise(at(0), maxed)
	require.Len(t, r.tick(at(DefaultSettle)), 1)

	r.clear(at(5*time.Minute), maxed)
	r.tick(at(5 * time.Minute))
	_, wake := r.m.Tick(at(5*time.Minute + DefaultHold + time.Second))
	assert.Equal(t, StillBrokenMax-DefaultHold-time.Second, wake,
		"a timer is armed for the end of the extra hold")

	wantNone(t, r.tick(at(5*time.Minute+StillBrokenMax-time.Second)))
	ds := r.tick(at(5*time.Minute + StillBrokenMax))
	require.Len(t, ds, 1, "the hold is over, so the incident resolves")
	assert.Equal(t, Resolve, ds[0].Action)
}
