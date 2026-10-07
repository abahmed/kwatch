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

// waitingService is a LoadBalancer Service with no address since t0.
func waitingService(t *testing.T, class string) (
	*inventory.Model, inventory.EntityID,
) {
	t.Helper()
	m := newTestModel()
	svc := newID(kube.KindService, "shop", "web")
	attrs := map[string]inventory.Value{
		kube.AttrServiceType:  inventory.Text("LoadBalancer"),
		kube.AttrLoadBalancer: inventory.Bool(false),
	}
	if class != "" {
		attrs[kube.AttrLoadBalancerClass] = inventory.Text(class)
	}
	put(m, svc, t0, attrs)
	return m, svc
}

func waitFinding(t *testing.T, m *inventory.Model, svc inventory.EntityID,
	after time.Duration,
) []detection.Finding {
	t.Helper()
	eval := evaluate(Service{}, m, t0.Add(after), svc, nil)
	var out []detection.Finding
	for _, f := range eval.Findings {
		if f.Reason == reasons.LoadBalancerPending {
			out = append(out, f)
		}
	}
	return out
}

func TestLoadBalancerWaitQuotesTheLatestEvent(t *testing.T) {
	m, svc := waitingService(t, "")
	m.Apply(inventory.Observation{
		Kind: inventory.Noted, Source: "test", At: t0, Entity: svc,
		Note: inventory.Note{At: t0, Reason: "EnsuringLoadBalancer",
			Message: "Ensuring load balancer", Count: 1},
	})
	warn(m, svc, t0.Add(2*time.Minute), "FailedDeployModel",
		"Failed to build model: subnets not found")

	got := waitFinding(t, m, svc, 40*time.Minute)

	require.Len(t, got, 1)
	assert.Equal(t, "has been waiting 40 min for a load balancer: "+
		"\"Failed to build model: subnets not found\"", got[0].Summary)
	assert.Contains(t, got[0].Evidence, detection.Evidence{
		Label: "FailedDeployModel",
		Value: "Failed to build model: subnets not found"})
}

func TestLoadBalancerWaitWithoutEventsSaysNoControllerActed(t *testing.T) {
	m, svc := waitingService(t, "")

	got := waitFinding(t, m, svc, 12*time.Minute)

	require.Len(t, got, 1)
	assert.Equal(t, "has been waiting 12 min for a load balancer: no "+
		"controller has acted on it", got[0].Summary)
}

func TestLoadBalancerWaitNamesTheClass(t *testing.T) {
	m, svc := waitingService(t, "example.com/internal-lb")

	got := waitFinding(t, m, svc, 12*time.Minute)

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "no controller for load balancer "+
		"class example.com/internal-lb has acted on it")
}

func TestLoadBalancerWaitMakesNoClaimOnceEventsExpired(t *testing.T) {
	m, svc := waitingService(t, "")

	got := waitFinding(t, m, svc, 3*time.Hour)

	require.Len(t, got, 1)
	assert.NotContains(t, got[0].Summary, "no controller")
}

func TestLoadBalancerWaitTierFollowsImpact(t *testing.T) {
	m, svc := waitingService(t, "")
	assert.Equal(t, detection.Info, waitFinding(t, m, svc, 12*time.Minute)[0].
		Severity, "a brand-new Service waits in the digest")

	ing := newID(kube.KindIngress, "shop", "web")
	put(m, ing, t0, nil)
	link(m, ing, inventory.RoutesTo, svc)
	assert.Equal(t, detection.Warning, waitFinding(t, m, svc,
		12*time.Minute)[0].Severity, "an Ingress depends on it")
}

func TestLoadBalancerWaitAfterLosingItsAddressNotifies(t *testing.T) {
	m, svc := waitingService(t, "")
	put(m, svc, t0.Add(time.Minute), map[string]inventory.Value{
		kube.AttrServiceType:  inventory.Text("LoadBalancer"),
		kube.AttrLoadBalancer: inventory.Bool(true),
	})
	put(m, svc, t0.Add(time.Hour), map[string]inventory.Value{
		kube.AttrServiceType:  inventory.Text("LoadBalancer"),
		kube.AttrLoadBalancer: inventory.Bool(false),
	})

	got := waitFinding(t, m, svc, time.Hour+12*time.Minute)

	require.Len(t, got, 1)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Equal(t, t0.Add(time.Hour), got[0].Since)
}

func TestLoadBalancerWaitCountsFromTheTypeChange(t *testing.T) {
	m := newTestModel()
	svc := newID(kube.KindService, "shop", "web")
	put(m, svc, t0, map[string]inventory.Value{
		kube.AttrServiceType:  inventory.Text("ClusterIP"),
		kube.AttrLoadBalancer: inventory.Bool(false),
	})
	put(m, svc, t0.Add(time.Hour), map[string]inventory.Value{
		kube.AttrServiceType:  inventory.Text("LoadBalancer"),
		kube.AttrLoadBalancer: inventory.Bool(false),
	})

	assert.Empty(t, waitFinding(t, m, svc, time.Hour+2*time.Minute))
	got := waitFinding(t, m, svc, time.Hour+12*time.Minute)
	require.Len(t, got, 1)
	assert.Equal(t, t0.Add(time.Hour), got[0].Since)
}

// The waiting finding quotes the controller's FailedDeployModel, so the
// unusual-event finding for it would only repeat it.
func TestFailedDeployModelIsNotAlsoAnUnusualEvent(t *testing.T) {
	m, svc := waitingService(t, "")
	for i := range 3 {
		warn(m, svc, t0.Add(time.Duration(i)*time.Minute),
			"FailedDeployModel", "Failed to build model: no subnets")
		warn(m, svc, t0.Add(time.Duration(i)*time.Minute),
			"FailedBuildModel", "Failed build model: no subnets")
	}

	eval := evaluate(Service{}, m, t0.Add(12*time.Minute), svc, nil)
	for _, f := range eval.Findings {
		assert.NotEqual(t, reasons.UnusualEvent("FailedDeployModel"),
			f.Reason)
		assert.NotEqual(t, reasons.UnusualEvent("FailedBuildModel"),
			f.Reason)
	}
}

func TestLoadBalancerWaitIsShorterOnceTheControllerFailed(t *testing.T) {
	m, svc := waitingService(t, "")
	assert.Empty(t, waitFinding(t, m, svc, 3*time.Minute))

	warn(m, svc, t0.Add(time.Minute), "FailedBuildModel", "no subnets")

	assert.Empty(t, waitFinding(t, m, svc, time.Minute+30*time.Second))
	assert.Len(t, waitFinding(t, m, svc, 3*time.Minute), 1)
}
