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

// backendRig is a Service with an unready endpoint slice and one
// not-ready pod it selects, on a node of the given age at t0.
func backendRig(nodeAge time.Duration) (
	*inventory.Model, inventory.EntityID, inventory.EntityID,
) {
	model, pod := replacementRig(nodeAge)
	put(model, pod, t0, map[string]inventory.Value{
		kube.AttrLabels:  inventory.Text("app=api"),
		kube.AttrCreated: inventory.Time(t0.Add(-nodeAge))})
	svc := newID(kube.KindService, "shop", "api")
	put(model, svc, t0, map[string]inventory.Value{
		kube.AttrServiceType: inventory.Text("ClusterIP"),
		kube.AttrSelector:    inventory.Text("app=api")})
	slice := newID(kube.KindEndpointSlice, "shop", "api-1")
	put(model, slice, t0, map[string]inventory.Value{
		kube.AttrEndpoints:      inventory.Number(1),
		kube.AttrEndpointsReady: inventory.Number(0)})
	link(model, slice, inventory.Backs, svc)
	return model, svc, pod
}

func TestServiceNoEndpointsWaitsForStartingPods(t *testing.T) {
	model, svc, _ := backendRig(0)

	early := evaluate(Service{}, model, t0.Add(4*time.Minute), svc, nil)
	assert.Empty(t, early.Findings)
	assert.Positive(t, early.RecheckAfter)

	late := evaluate(Service{}, model, t0.Add(kube.BootWindow), svc, nil)
	require.Len(t, late.Findings, 1)
	assert.Equal(t, reasons.ServiceNoEndpoints, late.Findings[0].Reason)
}

func TestServiceNoEndpointsKeepsCrashLoopingPods(t *testing.T) {
	model, svc, pod := backendRig(0)
	container := newID(kube.KindContainer, "shop", "web-1/app")
	put(model, container, t0, map[string]inventory.Value{
		kube.AttrRestarts: inventory.Number(2)})
	relateEntity(model, container, inventory.PartOf, pod)

	got := evaluate(Service{}, model, t0.Add(2*time.Minute), svc, nil)
	require.Len(t, got.Findings, 1)
	assert.Equal(t, reasons.ServiceNoEndpoints, got.Findings[0].Reason)
}

func TestServiceNoEndpointsReportsOldNotReadyPods(t *testing.T) {
	model, svc, _ := backendRig(time.Hour)

	got := evaluate(Service{}, model, t0.Add(2*time.Minute), svc, nil)
	require.Len(t, got.Findings, 1)
}

func TestWebhookNoEndpointsWaitsForStartingPods(t *testing.T) {
	model, svc, _ := backendRig(0)
	hook := newID(kube.KindMutatingWebhook, "", "datadog")
	put(model, hook, t0, map[string]inventory.Value{
		kube.AttrFailurePolicy: inventory.Text("Ignore")})
	link(model, hook, inventory.Serves, svc)

	early := evaluate(Webhook{}, model, t0.Add(4*time.Minute), hook, nil)
	assert.Empty(t, early.Findings)
	late := evaluate(Webhook{}, model, t0.Add(kube.BootWindow), hook, nil)
	require.Len(t, late.Findings, 1)
	assert.Equal(t, reasons.WebhookNoEndpoints, late.Findings[0].Reason)
}

func TestNodeCommitmentWaitsForYoungNode(t *testing.T) {
	m, node := commitmentModel(1000, 800, 800)
	put(m, node, t0, conditionAttrs(map[string]inventory.Value{
		kube.AttrMemoryAllocatable: inventory.Number(1000),
		kube.AttrCreated:           inventory.Time(t0)},
		"MemoryPressure", "True", "KubeletHasInsufficientMemory", t0))

	early := evaluate(NodeCommitment{}, m, t0.Add(5*time.Minute), node, nil)
	assert.Empty(t, early.Findings)
	assert.Equal(t, 5*time.Minute, early.RecheckAfter)
	registry := detection.NewRegistry(nil, NodeCommitment{})
	registry.Evaluate(m, t0.Add(kube.BootWindow), node)
	late := registry.Evaluate(m,
		t0.Add(kube.BootWindow+overcommitSustain), node)
	assert.Len(t, late.Findings, 1)
}

func TestNodePressureStallWaitsForYoungNode(t *testing.T) {
	m := newTestModel()
	node := newID(kube.KindNode, "", "n1")
	put(m, node, t0, map[string]inventory.Value{
		kube.AttrCreated:   inventory.Time(t0),
		kube.AttrMemoryPSI: inventory.Number(40)})

	early := evaluate(NodeUsage{}, m, t0.Add(5*time.Minute), node, nil)
	assert.Empty(t, early.Findings)
	late := evaluate(NodeUsage{}, m, t0.Add(kube.BootWindow), node, nil)
	require.Len(t, late.Findings, 1)
	assert.Equal(t, reasons.NodePSIHigh, late.Findings[0].Reason)
}
