package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestEventSandboxFailureThatTheRetryOvercomesIsNotReported(
	t *testing.T,
) {
	model, pod := replacementRig(time.Hour)
	noteEntity(model, pod, "FailedCreatePodSandBox", cniAddFailed, 1, t0)
	detect := func(now time.Time) []string {
		ctx := testDetectorContext(model, now)
		return findingReasons(Event{}.Detect(ctx, entityOf(model, pod)))
	}

	assert.Empty(t, detect(t0.Add(time.Second)),
		"the kubelet retries a sandbox at once")
	put(model, pod, t0.Add(2*time.Second), map[string]inventory.Value{
		kube.AttrContainersStarted: inventory.Time(t0.Add(time.Second)),
	})
	assert.Empty(t, detect(t0.Add(DefaultSandboxGrace)),
		"the retry worked: the event is history")
}

func TestEventSandboxFailureThatLastsIsReported(t *testing.T) {
	model, pod := replacementRig(time.Hour)
	noteEntity(model, pod, "FailedCreatePodSandBox", cniAddFailed, 1, t0)
	ctx := testDetectorContext(model, t0.Add(time.Second))
	Event{}.Detect(ctx, entityOf(model, pod))

	ctx = testDetectorContext(model, t0.Add(DefaultSandboxGrace))
	assert.Contains(t, findingReasons(Event{}.Detect(ctx,
		entityOf(model, pod))), "FailedCreatePodSandBox")
}

func TestEventSandboxFailureThatTheRetryAlsoHitIsReportedAtOnce(
	t *testing.T,
) {
	model, pod := replacementRig(time.Hour)
	noteEntity(model, pod, "FailedCreatePodSandBox", cniAddFailed, 2, t0)
	ctx := testDetectorContext(model, t0.Add(time.Second))

	assert.Contains(t, findingReasons(Event{}.Detect(ctx,
		entityOf(model, pod))), "FailedCreatePodSandBox")
}
