package detectors

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
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

func TestAddressAssignmentOnAYoungNodeWaitsForTheNodeToAge(t *testing.T) {
	model, pod := replacementRig(time.Minute)
	noteEntity(model, pod, "FailedCreatePodSandBox",
		"plugin failed (add): failed to assign an IP address", 1, t0)
	detect := func(now time.Time) []string {
		ctx := testDetectorContext(model, now)
		return findingReasons(Event{}.Detect(ctx, entityOf(model, pod)))
	}

	assert.Empty(t, detect(t0.Add(time.Minute)),
		"a young node's address pool is still warming up")
	assert.Contains(t, detect(t0.Add(kube.BootWindow)),
		"FailedCreatePodSandBox",
		"a pool still empty once the node is old is reported")
}

func TestNodeSandboxFailuresWaitWhileTheNodeIsYoung(t *testing.T) {
	model := newTestModel()
	sandboxPods(model, ipExhaustedMessage, ipExhaustedMessage,
		ipExhaustedMessage)
	observeEntity(model, podNodeID, podNodeNow, map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(podNodeNow.Add(-2 * time.Minute)),
	})
	registry := detection.NewRegistry(nil, NewNode(0))

	got := registry.Evaluate(model, podNodeNow, podNodeID)

	assert.Empty(t, got.Findings)
	assert.LessOrEqual(t, got.RecheckAfter, kube.BootWindow-2*time.Minute)
}

func TestNodeSandboxFailuresOfPodsThatStartedSinceAreOver(t *testing.T) {
	model := newTestModel()
	sandboxPods(model, ipExhaustedMessage, ipExhaustedMessage,
		ipExhaustedMessage)
	for i := range 3 {
		id := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: fmt.Sprintf("app-%d", i)}
		observeEntity(model, id, podNodeNow, map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Running"),
			kube.AttrReady: inventory.Bool(true),
		})
	}

	assert.Empty(t, findingReasons(detectNode(model)),
		"every pod was retried and runs: nothing fails now")
}
