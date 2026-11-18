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

// deployedWorkload adds a Deployment of release payments in shop that
// wants two replicas and has ready of them ready.
func deployedWorkload(m *inventory.Model, name string, ready float64) {
	id := newID(kube.KindDeployment, "shop", name)
	put(m, id, t0, map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(2),
		kube.AttrReadyReplicas: inventory.Number(ready),
	})
	m.Apply(inventory.Observation{Kind: inventory.Related, Source: "test",
		At: t0, Entity: id, Relation: inventory.DeployedBy,
		Targets: []inventory.EntityID{
			kube.HelmReleaseID("shop", "payments")}})
}

// failedSeverity is the severity of release payments, failed age ago.
func failedSeverity(
	t *testing.T, age time.Duration, workloads func(*inventory.Model),
) detection.Severity {
	t.Helper()
	m, id := helmModel("failed", age)
	if workloads != nil {
		workloads(m)
	}
	got := detectHelm(m, id)
	require.Len(t, got, 1)
	assert.Equal(t, "HelmReleaseFailed", got[0].Reason)
	return got[0].Severity
}

func TestOldHelmFailureWithHealthyWorkloadsIsLow(t *testing.T) {
	old := DefaultHelmFailureOld + 24*time.Hour
	assert.Equal(t, detection.Info, failedSeverity(t, old,
		func(m *inventory.Model) { deployedWorkload(m, "api", 2) }))
	assert.Equal(t, detection.Info, failedSeverity(t, old, nil),
		"a release whose workloads kwatch does not see is old news too")
}

func TestHelmFailureStaysWarningWhenRecentOrBreaking(t *testing.T) {
	old := DefaultHelmFailureOld + 24*time.Hour
	assert.Equal(t, detection.Warning, failedSeverity(t,
		DefaultHelmFailureOld-time.Hour,
		func(m *inventory.Model) { deployedWorkload(m, "api", 2) }),
		"a recent failure")
	assert.Equal(t, detection.Warning, failedSeverity(t, old,
		func(m *inventory.Model) {
			deployedWorkload(m, "api", 2)
			deployedWorkload(m, "worker", 1)
		}), "a workload of the release is short of ready replicas")
}

func TestOldStuckHelmReleaseStaysWarning(t *testing.T) {
	m, id := helmModel("pending-upgrade", DefaultHelmFailureOld+time.Hour)
	got := detectHelm(m, id)
	require.Len(t, got, 1)
	assert.Equal(t, detection.Warning, got[0].Severity)
}

func TestRecentHelmFailureIsJudgedAgainWhenOld(t *testing.T) {
	m, id := helmModel("failed", time.Hour)
	r := detection.NewRegistry(nil, NewHelmRelease(0))
	result := r.Evaluate(m, t0, id)
	assert.Equal(t, DefaultHelmFailureOld-time.Hour+time.Second,
		result.RecheckAfter)
}

func TestOldHelmFailureWithAnUnreadyPodStaysWarning(t *testing.T) {
	old := DefaultHelmFailureOld + 24*time.Hour
	severity := func(unready time.Duration) detection.Severity {
		return failedSeverity(t, old, func(m *inventory.Model) {
			deployedWorkload(m, "api", 2)
			pod := newID(kube.KindPod, "shop", "api-1")
			put(m, pod, t0, map[string]inventory.Value{
				kube.AttrReady:      inventory.Bool(false),
				kube.AttrReadySince: inventory.Time(t0.Add(-unready)),
			})
			m.Apply(inventory.Observation{Kind: inventory.Related,
				Source: "test", At: t0, Entity: pod,
				Relation: inventory.OwnedBy, Targets: []inventory.EntityID{
					newID(kube.KindDeployment, "shop", "api")}})
		})
	}
	assert.Equal(t, detection.Warning, severity(10*time.Minute),
		"a pod unready for ten minutes: a workload down")
	assert.Equal(t, detection.Info, severity(time.Minute),
		"a pod starting is not a workload down")
}
