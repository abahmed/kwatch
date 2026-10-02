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

func detectEventNote(
	id inventory.EntityID, reason, message string, warning bool,
) []detection.Finding {
	model := newTestModel()
	observeEntity(model, id, podNodeNow, map[string]inventory.Value{})
	model.Apply(inventory.Observation{
		Kind: inventory.Noted, Source: "test", At: podNodeNow, Entity: id,
		Note: inventory.Note{
			At: podNodeNow.Add(-time.Minute), Source: "kubelet",
			Reason: reason, Message: message, Count: 1, Warning: warning,
		},
	})
	registry := detection.NewRegistry(nil, Event{})
	return registry.Evaluate(model, podNodeNow, id).Findings
}

func TestEventKubeletNodeModes(t *testing.T) {
	node := inventory.EntityID{Kind: kube.KindNode, Name: "worker-1"}
	cases := []struct {
		reason, message, mode string
	}{
		{reasons.EvictionThresholdMet, "Attempting to reclaim " +
			"ephemeral-storage", "Disk.EvictionThreshold"},
		{reasons.EvictionThresholdMet, "Attempting to reclaim inodes",
			"Disk.EvictionThreshold"},
		{reasons.EvictionThresholdMet, "Attempting to reclaim memory",
			"Memory.EvictionThreshold"},
		{reasons.EvictionThresholdMet, "Attempting to reclaim pids",
			"PID.EvictionThreshold"},
		{reasons.ImageGCFailed, "failed to garbage collect required " +
			"amount of images", "Disk.ImageGCFailed"},
		{reasons.FreeDiskSpaceFailed, "Failed to garbage collect " +
			"required amount of images", "Disk.FreeSpaceFailed"},
		{reasons.ContainerGCFailed, "rpc error: runtime unavailable",
			"Disk.ContainerGCFailed"},
	}
	for _, tc := range cases {
		found := detectEventNote(node, tc.reason, tc.message, true)
		require.Len(t, found, 1, tc.message)
		assert.Equal(t, tc.mode, string(found[0].Mode), tc.message)
		assert.Equal(t, detection.Degraded, found[0].Health)
		assert.NotContains(t, found[0].Summary, "Kubernetes reported")
	}
	assert.Empty(t, detectEventNote(node, reasons.ImageGCFailed, "x",
		false), "normal events are not failures")
	gc := detectEventNote(node, reasons.ImageGCFailed, "x", true)
	require.Len(t, gc, 1)
	assert.Equal(t, detection.Info, gc[0].Severity,
		"a failed image GC alone is digest material")
}

func TestEventSandboxModes(t *testing.T) {
	pod := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "app"}
	runtimeNotReady := "network is not ready: container runtime " +
		"network not ready: NetworkReady=false reason:" +
		"NetworkPluginNotReady message:Network plugin returns error: " +
		"cni plugin not initialized"
	cases := map[string]string{
		ipExhaustedMessage: "Network.IPExhausted",
		cniNotReadyMessage: "Network.CNINotReady",
		runtimeNotReady:    "Network.CNINotReady",
		"Failed to create pod sandbox: failed to pull image": "" +
			"FailedCreatePodSandBox",
	}
	for message, mode := range cases {
		found := detectEventNote(pod, "FailedCreatePodSandBox", message,
			true)
		require.Len(t, found, 1)
		assert.Equal(t, mode, string(found[0].Mode), message)
		assert.Equal(t, "FailedCreatePodSandBox", found[0].Reason,
			"the reason stays for root-cause rules")
	}
}
