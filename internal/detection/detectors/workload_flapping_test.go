package detectors

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

const flapProbeMessage = "Readiness probe failed: Get " +
	"\"http://10.0.1.5:8080/ready\": context deadline exceeded"

// flapRig is a Deployment of two running pods, ready since t0.
func flapRig() (*inventory.Model, inventory.EntityID, []inventory.EntityID) {
	model := newTestModel()
	deploy := newID(kube.KindDeployment, "shop", "api")
	put(model, deploy, t0, map[string]inventory.Value{
		kube.AttrReplicas: inventory.Number(2)})
	var pods []inventory.EntityID
	for _, name := range []string{"api-0", "api-1"} {
		pod := newID(kube.KindPod, "shop", name)
		put(model, pod, t0, map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Running"),
			kube.AttrReady: inventory.Bool(true),
			kube.AttrCreated: inventory.Time(
				t0.Add(-time.Hour))})
		relateEntity(model, pod, inventory.OwnedBy, deploy)
		pods = append(pods, pod)
	}
	return model, deploy, pods
}

// toggle flips the pod's Ready n times, once every step from start.
func toggle(
	model *inventory.Model, pod inventory.EntityID, start time.Time,
	step time.Duration, n int,
) {
	ready := true
	for i := range n {
		ready = !ready
		put(model, pod, start.Add(time.Duration(i)*step),
			map[string]inventory.Value{
				kube.AttrPhase: inventory.Text("Running"),
				kube.AttrReady: inventory.Bool(ready),
				kube.AttrCreated: inventory.Value(inventory.Time(
					t0.Add(-time.Hour)))})
	}
}

func flappingOf(found []detection.Finding) (detection.Finding, bool) {
	for _, f := range found {
		if f.Reason == reasons.ReadinessFlapping {
			return f, true
		}
	}
	return detection.Finding{}, false
}

func detectFlap(
	model *inventory.Model, id inventory.EntityID, at time.Time,
) []detection.Finding {
	return NewWorkload(0).Detect(testDetectorContext(model, at),
		entityOf(model, id))
}

func TestReadinessFlappingAcrossThePods(t *testing.T) {
	model, deploy, pods := flapRig()
	toggle(model, pods[0], t0.Add(time.Minute), 30*time.Second, 5)
	toggle(model, pods[1], t0.Add(90*time.Second), 30*time.Second, 5)
	warn(model, pods[0], t0.Add(time.Minute), "Unhealthy", flapProbeMessage)

	f, ok := flappingOf(detectFlap(model, deploy, t0.Add(5*time.Minute)))

	require.True(t, ok)
	assert.Equal(t, detection.Warning, f.Severity)
	assert.Contains(t, f.Summary, "went ready to unready 6 times")
	assert.Contains(t, f.Summary, "last 10 min")
	assert.Contains(t, f.Evidence, detection.Evidence{
		Label: "readiness probe", Value: flapProbeMessage})
	assert.Equal(t, detection.ModeReadinessFlapping,
		detection.Classify(f).Mode)
}

func TestReadinessFlappingNeedsEnoughFlips(t *testing.T) {
	model, deploy, pods := flapRig()
	toggle(model, pods[0], t0.Add(time.Minute), 30*time.Second, 5)

	_, ok := flappingOf(detectFlap(model, deploy, t0.Add(5*time.Minute)))

	assert.False(t, ok, "five transitions are not flapping")
}

func TestReadinessFlappingForgetsOldFlips(t *testing.T) {
	model, deploy, pods := flapRig()
	toggle(model, pods[0], t0.Add(time.Minute), 30*time.Second, 8)

	_, now := flappingOf(detectFlap(model, deploy, t0.Add(6*time.Minute)))
	_, later := flappingOf(detectFlap(model, deploy, t0.Add(20*time.Minute)))

	assert.True(t, now)
	assert.False(t, later, "the flips left the window")
}

func TestReadinessFlappingAsksToBeChecked(t *testing.T) {
	model, deploy, pods := flapRig()
	toggle(model, pods[0], t0.Add(time.Minute), 30*time.Second, 8)

	got := evaluate(NewWorkload(0), model, t0.Add(6*time.Minute), deploy,
		nil)

	assert.Positive(t, got.RecheckAfter, "the finding must end by itself")
}

func TestReadinessFlappingIgnoresFirstReady(t *testing.T) {
	model := newTestModel()
	deploy := newID(kube.KindDeployment, "shop", "api")
	put(model, deploy, t0, map[string]inventory.Value{
		kube.AttrReplicas: inventory.Number(8)})
	// Eight pods start: each is seen not ready, then becomes ready.
	for i := range 8 {
		pod := newID(kube.KindPod, "shop", "api-"+string(rune('a'+i)))
		attrs := func(ready bool) map[string]inventory.Value {
			return map[string]inventory.Value{
				kube.AttrPhase:   inventory.Text("Running"),
				kube.AttrReady:   inventory.Bool(ready),
				kube.AttrCreated: inventory.Time(t0)}
		}
		put(model, pod, t0, attrs(false))
		relateEntity(model, pod, inventory.OwnedBy, deploy)
		put(model, pod, t0.Add(40*time.Second), attrs(true))
	}

	_, ok := flappingOf(detectFlap(model, deploy, t0.Add(2*time.Minute)))

	assert.False(t, ok, "a first Ready is startup, not flapping")
}

func TestReadinessFlappingIgnoresRestartingPods(t *testing.T) {
	model, deploy, pods := flapRig()
	toggle(model, pods[0], t0.Add(time.Minute), 30*time.Second, 8)
	container := newID(kube.KindContainer, "shop", "api-0/app")
	put(model, container, t0.Add(2*time.Minute), map[string]inventory.Value{
		kube.AttrRestarts: inventory.Number(3)})
	relateEntity(model, container, inventory.PartOf, pods[0])

	_, ok := flappingOf(detectFlap(model, deploy, t0.Add(5*time.Minute)))

	assert.False(t, ok, "a pod that restarts is crashing, not flapping")
}

func TestReadinessFlappingIsUnusualForASteadyWorkload(t *testing.T) {
	model, deploy, pods := flapRig()
	for hour := range 12 {
		model.Baselines().Add(deploy,
			inventory.WarningMetric(unhealthyEvent), 0,
			t0.Add(time.Duration(hour-12)*time.Hour))
	}
	toggle(model, pods[0], t0.Add(time.Minute), 30*time.Second, 8)
	model.Apply(inventory.Observation{Kind: inventory.Noted,
		Source: "test", At: t0.Add(2 * time.Minute), Entity: pods[0],
		Note: inventory.Note{At: t0.Add(2 * time.Minute),
			Reason: "Unhealthy", Message: flapProbeMessage, Count: 20,
			Warning: true}})

	f, ok := flappingOf(detectFlap(model, deploy, t0.Add(5*time.Minute)))

	require.True(t, ok)
	assert.Equal(t, detection.NormalUnusual, f.Normal)
}

func TestReadinessFlappingIgnoresARestartCountThatWasReset(t *testing.T) {
	model, deploy, pods := flapRig()
	toggle(model, pods[0], t0.Add(time.Minute), 30*time.Second, 8)
	container := newID(kube.KindContainer, "shop", "api-0/app")
	put(model, container, t0.Add(-time.Hour), map[string]inventory.Value{
		kube.AttrRestarts: inventory.Number(0)})
	relateEntity(model, container, inventory.PartOf, pods[0])
	put(model, container, t0.Add(3*time.Minute), map[string]inventory.Value{
		kube.AttrRestarts: inventory.Number(7)})
	put(model, container, t0.Add(4*time.Minute), map[string]inventory.Value{
		kube.AttrRestarts: inventory.Number(0)})

	_, ok := flappingOf(detectFlap(model, deploy, t0.Add(5*time.Minute)))

	assert.False(t, ok, "a restart count that moved is a restarting pod")
}
