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

func TestServiceClusterIPReady(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-10 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindService, Namespace: "default",
		Name: "api"}
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrServiceType: inventory.Text("ClusterIP"),
			kube.AttrSelector:    inventory.Text("app=api"),
		},
	})

	entity, _ := model.Entity(id)
	detector := Service{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	assert.Empty(t, findings)
}

func TestServiceLoadBalancerPending(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-7 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindService, Namespace: "default",
		Name: "api"}
	model.Apply(inventory.Observation{
		Kind:   inventory.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrServiceType:  inventory.Text("LoadBalancer"),
			kube.AttrLoadBalancer: inventory.Bool(false),
		},
	})

	entity, _ := model.Entity(id)
	detector := Service{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.LoadBalancerPending, findings[0].Reason)
	assert.Equal(t, detection.Warning, findings[0].Severity)
}

func TestServiceExternalName(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-10 * time.Minute)

	service := buildNetwork(
		kube.KindService, "external", "default", since,
		map[string]inventory.Value{
			kube.AttrServiceType: inventory.Text("ExternalName"),
		},
	)

	detector := Service{}
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, service)

	assert.Empty(t, findings)
}

func TestIngressMissingBackend(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindIngress, Namespace: "default",
		Name: "api"}
	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]inventory.Value{},
	})

	entity, _ := model.Entity(id)
	detector := Ingress{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	// No findings expected if no missing services are found
	assert.Empty(t, findings)
}

func TestNetworkPolicyEgress(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-10 * time.Minute)

	policy := buildNetwork(
		kube.KindNetworkPolicy, "deny-all", "default", since,
		map[string]inventory.Value{
			kube.AttrDeniesEgress: inventory.Bool(true),
		},
	)

	detector := EgressPolicy{}
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, policy)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.RestrictiveNetworkPolicy,
		findings[0].Reason)
	assert.Equal(t, detection.Info, findings[0].Severity)
}
