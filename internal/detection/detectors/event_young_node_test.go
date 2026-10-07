package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const cniAddFailed = "failed to setup network for sandbox: plugin " +
	"type=\"aws-cni\" failed (add): connection refused"

func TestSandboxEventOnAYoungNodeWaitsForTheNodeToAge(t *testing.T) {
	model, pod := replacementRig(time.Minute)
	noteEntity(model, pod, "FailedCreatePodSandBox", cniAddFailed, 1, t0)
	detect := func(now time.Time) []string {
		ctx := testDetectorContext(model, now)
		return findingReasons(Event{}.Detect(ctx, entityOf(model, pod)))
	}

	assert.Empty(t, detect(t0.Add(time.Minute)),
		"a node that just joined is still starting its network plugin")
	require.Contains(t, detect(t0.Add(kube.BootWindow)),
		"FailedCreatePodSandBox",
		"a pod that still cannot start once the node is old is reported")
}

func TestSandboxEventOnAnOldNodeIsReported(t *testing.T) {
	model, pod := replacementRig(time.Hour)
	noteEntity(model, pod, "FailedCreatePodSandBox", cniAddFailed, 1, t0)
	ctx := testDetectorContext(model, t0.Add(time.Minute))

	assert.Contains(t, findingReasons(Event{}.Detect(ctx,
		entityOf(model, pod))), "FailedCreatePodSandBox")
}

func TestExhaustedAddressesOnAYoungNodeAreReported(t *testing.T) {
	model, pod := replacementRig(time.Minute)
	noteEntity(model, pod, "FailedCreatePodSandBox",
		"plugin failed (add): failed to assign an IP address", 1, t0)
	ctx := testDetectorContext(model, t0.Add(time.Minute))

	assert.Contains(t, findingReasons(Event{}.Detect(ctx,
		entityOf(model, pod))), "FailedCreatePodSandBox",
		"an exhausted pool does not heal with the node's age")
}
