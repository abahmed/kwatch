package detect

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestServiceClusterIPReady(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-10 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindService, Namespace: "default",
		Name: "api"}
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrServiceType: knowledge.Text("ClusterIP"),
			kube.AttrSelector:    knowledge.Text("app=api"),
		},
	})

	entity, _ := model.Entity(id)
	detector := Service{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	assert.Empty(t, signals)
}

func TestServiceLoadBalancerPending(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-7 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindService, Namespace: "default",
		Name: "api"}
	model.Apply(knowledge.Fact{
		Kind:   knowledge.Observed,
		Source: "test",
		At:     since,
		Entity: id,
		Attributes: map[string]knowledge.Value{
			kube.AttrServiceType:  knowledge.Text("LoadBalancer"),
			kube.AttrLoadBalancer: knowledge.Bool(false),
		},
	})

	entity, _ := model.Entity(id)
	detector := Service{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonLoadBalancerPending, signals[0].Reason)
	assert.Equal(t, signal.Warning, signals[0].Severity)
}

func TestServiceExternalName(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-10 * time.Minute)

	service := buildNetwork(
		kube.KindService, "external", "default", since,
		map[string]knowledge.Value{
			kube.AttrServiceType: knowledge.Text("ExternalName"),
		},
	)

	detector := Service{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, service)

	assert.Empty(t, signals)
}

func TestIngressMissingBackend(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-1 * time.Minute)

	model := newTestModel()
	id := knowledge.EntityID{Kind: kube.KindIngress, Namespace: "default",
		Name: "api"}
	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         since,
		Entity:     id,
		Attributes: map[string]knowledge.Value{},
	})

	entity, _ := model.Entity(id)
	detector := Ingress{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	// No signals expected if no missing services are found
	assert.Empty(t, signals)
}

func TestNetworkPolicyEgress(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-10 * time.Minute)

	policy := buildNetwork(
		kube.KindNetworkPolicy, "deny-all", "default", since,
		map[string]knowledge.Value{
			kube.AttrDeniesEgress: knowledge.Bool(true),
		},
	)

	detector := EgressPolicy{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, policy)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonRestrictiveNetworkPolicy,
		signals[0].Reason)
	assert.Equal(t, signal.Info, signals[0].Severity)
}
