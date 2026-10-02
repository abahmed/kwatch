package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestServiceReportsLoadBalancerSyncFailure(t *testing.T) {
	m := newTestModel()
	svc := newID(kube.KindService, "ns", "web")
	put(m, svc, t0, map[string]inventory.Value{
		kube.AttrServiceType:  inventory.Text("LoadBalancer"),
		kube.AttrLoadBalancer: inventory.Bool(false),
	})
	warn(m, svc, t0, "SyncLoadBalancerFailed",
		"Error syncing load balancer: quota exceeded")
	warn(m, svc, t0.Add(time.Minute), "UpdateLoadBalancerFailed",
		"Error updating load balancer with new hosts")

	eval := evaluate(Service{}, m, t0.Add(2*time.Minute), svc, nil)

	require.Len(t, eval.Findings, 1)
	f := eval.Findings[0]
	assert.Equal(t, "LoadBalancer.SyncFailed", string(f.Mode))
	assert.Equal(t, detection.Critical, f.Severity)
	assert.Equal(t, t0, f.Since)
	assert.Contains(t, f.Evidence, detection.Evidence{
		Label: "SyncLoadBalancerFailed",
		Value: "Error syncing load balancer: quota exceeded"})
}

func TestServiceIgnoresNormalAndStaleLoadBalancerEvents(t *testing.T) {
	m := newTestModel()
	svc := newID(kube.KindService, "ns", "web")
	put(m, svc, t0, map[string]inventory.Value{
		kube.AttrServiceType: inventory.Text("ClusterIP"),
	})
	warn(m, svc, t0, "DeleteLoadBalancerFailed", "Error deleting")
	m.Apply(inventory.Observation{
		Kind: inventory.Noted, Source: "test", At: t0, Entity: svc,
		Note: inventory.Note{At: t0, Reason: "EnsuredLoadBalancer"},
	})

	stale := evaluate(Service{}, m, t0.Add(time.Hour), svc, nil)
	assert.Empty(t, stale.Findings)
}

// TestServiceIgnoresLoadBalancerFailureOnceAddressed: a failure older
// than the address no longer matters; a newer one still does.
func TestServiceIgnoresLoadBalancerFailureOnceAddressed(t *testing.T) {
	m := newTestModel()
	svc := newID(kube.KindService, "ns", "web")
	put(m, svc, t0, map[string]inventory.Value{
		kube.AttrServiceType:  inventory.Text("LoadBalancer"),
		kube.AttrLoadBalancer: inventory.Bool(false),
	})
	warn(m, svc, t0, "SyncLoadBalancerFailed", "quota exceeded")
	put(m, svc, t0.Add(time.Minute), map[string]inventory.Value{
		kube.AttrServiceType:  inventory.Text("LoadBalancer"),
		kube.AttrLoadBalancer: inventory.Bool(true),
	})
	now := t0.Add(2 * time.Minute)

	assert.Empty(t, evaluate(Service{}, m, now, svc, nil).Findings)

	warn(m, svc, t0.Add(90*time.Second), "UpdateLoadBalancerFailed",
		"hosts")
	assert.Len(t, evaluate(Service{}, m, now, svc, nil).Findings, 1)
}
