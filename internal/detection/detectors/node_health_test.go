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

var podNodeID = inventory.EntityID{Kind: kube.KindNode, Name: "worker-1"}

func detectNode(model *inventory.Model) []detection.Finding {
	return classified(NewNode(0).Detect(
		testDetectorContext(model, podNodeNow), entityOf(model, podNodeID)))
}

func TestNodeHeartbeatStale(t *testing.T) {
	model := newTestModel()
	observeEntity(model, podNodeID, podNodeNow, conditionAttrs(nil,
		"Ready", "Unknown", "NodeStatusUnknown",
		podNodeNow.Add(-time.Minute)))
	found := detectNode(model)
	require.Contains(t, findingReasons(found), reasons.NodeHeartbeatStale)
	for _, f := range found {
		if f.Reason == reasons.NodeHeartbeatStale {
			assert.Equal(t, "Heartbeat.Stale", string(f.Mode))
			assert.Equal(t, detection.Degraded, f.Health)
		}
	}
}

func TestNodeHeartbeatNotStale(t *testing.T) {
	cases := map[string]map[string]inventory.Value{
		"inside grace": conditionAttrs(nil, "Ready", "Unknown",
			"NodeStatusUnknown", podNodeNow.Add(-10*time.Second)),
		"kubelet reports not ready": conditionAttrs(nil, "Ready", "False",
			"KubeletNotReady", podNodeNow.Add(-time.Hour)),
		"ready": conditionAttrs(nil, "Ready", "True", "KubeletReady",
			podNodeNow.Add(-time.Hour)),
	}
	for name, attrs := range cases {
		model := newTestModel()
		observeEntity(model, podNodeID, podNodeNow, attrs)
		assert.NotContains(t, findingReasons(detectNode(model)),
			reasons.NodeHeartbeatStale, name)
	}
}

// sandboxPods schedules pods on the node, each with one sandbox failure.
func sandboxPods(model *inventory.Model, messages ...string) {
	observeEntity(model, podNodeID, podNodeNow, conditionAttrs(nil,
		"Ready", "True", "KubeletReady", podNodeNow.Add(-time.Hour)))
	for i, message := range messages {
		id := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: fmt.Sprintf("app-%d", i)}
		observeEntity(model, id, podNodeNow, map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Pending"),
		})
		relateEntity(model, id, inventory.RunsOn, podNodeID)
		noteEntity(model, id, "FailedCreatePodSandBox", message, 2,
			podNodeNow.Add(-time.Minute))
	}
}

const (
	ipExhaustedMessage = "Failed to create pod sandbox: plugin type=" +
		"\"host-local\" failed (add): failed to allocate for range 0: " +
		"no IP addresses available in range set: 10.244.1.1-10.244.1.254"
	cniNotReadyMessage = "Failed to create pod sandbox: rpc error: " +
		"network plugin is not ready: cni config uninitialized"
)

func TestNodeSandboxFailuresAggregate(t *testing.T) {
	cases := []struct {
		message, reason, mode string
	}{
		{ipExhaustedMessage, reasons.NodePodIPExhausted,
			"Network.IPExhausted"},
		{cniNotReadyMessage, reasons.NodeCNINotReady,
			"Network.CNINotReady"},
	}
	for _, tc := range cases {
		model := newTestModel()
		sandboxPods(model, tc.message, tc.message, tc.message)
		var node []detection.Finding
		for _, f := range detectNode(model) {
			if f.Reason == tc.reason {
				node = append(node, f)
			}
		}
		require.Len(t, node, 1, tc.reason)
		assert.Equal(t, tc.mode, string(node[0].Mode))
		assert.Equal(t, detection.Failing, node[0].Health)
		assert.Equal(t, "3", node[0].Evidence[0].Value)
	}
}

func TestNodeSandboxFailuresBelowThreshold(t *testing.T) {
	model := newTestModel()
	sandboxPods(model, ipExhaustedMessage, ipExhaustedMessage,
		cniNotReadyMessage, "Failed to create pod sandbox: image pull")
	registry := detection.NewRegistry(nil, NewNode(0))
	evaluation := registry.Evaluate(model, podNodeNow, podNodeID)
	assert.Empty(t, evaluation.Findings)
	assert.Equal(t, sandboxRecheck, evaluation.RecheckAfter,
		"pending pods keep the node under watch")
}

// The node's finding begins with the first failure of its pods, not
// with the latest retry: the pods were never failing before it.
func TestNodeSandboxFindingBeginsAtTheFirstFailure(t *testing.T) {
	model := newTestModel()
	sandboxPods(model, cniNotReadyMessage, cniNotReadyMessage,
		cniNotReadyMessage)
	first := podNodeNow.Add(-time.Minute)
	for i := range 3 {
		id := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
			Name: fmt.Sprintf("app-%d", i)}
		noteEntity(model, id, "FailedCreatePodSandBox",
			cniNotReadyMessage, 3, podNodeNow.Add(-10*time.Second))
	}
	var node []detection.Finding
	for _, f := range detectNode(model) {
		if f.Reason == reasons.NodeCNINotReady {
			node = append(node, f)
		}
	}
	require.Len(t, node, 1)
	assert.Equal(t, first, node[0].Since)
}
