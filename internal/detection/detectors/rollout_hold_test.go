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

// heldRolloutRig is a Deployment with three of four replicas ready whose
// newest ReplicaSet was created rolledAgo before t0.
func heldRolloutRig(
	reason, status string, rolledAgo time.Duration,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	dep := newID(kube.KindDeployment, "shop", "api")
	attrs := conditionAttrs(map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(4),
		kube.AttrReadyReplicas: inventory.Number(3),
	}, "Progressing", status, reason, t0.Add(-rolledAgo))
	put(m, dep, t0.Add(-rolledAgo), attrs)
	rs := newID(kube.KindReplicaSet, "shop", "api-new")
	put(m, rs, t0.Add(-rolledAgo), map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(t0.Add(-rolledAgo)),
	})
	link(m, rs, inventory.OwnedBy, dep)
	return m, dep
}

func heldRolloutFindings(
	m *inventory.Model, dep inventory.EntityID,
) []detection.Finding {
	return evaluate(NewWorkload(5*time.Minute), m, t0, dep, nil).Findings
}

func TestRolloutInProgressHoldsTheUnavailableFinding(t *testing.T) {
	m, dep := heldRolloutRig("ReplicaSetUpdated", "True", 9*time.Minute)
	assert.Empty(t, heldRolloutFindings(m, dep),
		"a replica short for nine minutes during a normal rollout")
}

func TestRolloutHoldEndsWhenTheControllerGivesUp(t *testing.T) {
	m, dep := heldRolloutRig("ProgressDeadlineExceeded", "False",
		12*time.Minute)
	reasonsSeen := map[string]bool{}
	for _, f := range heldRolloutFindings(m, dep) {
		reasonsSeen[f.Reason] = true
	}
	assert.True(t, reasonsSeen[reasons.ProgressDeadlineExceeded])
	assert.True(t, reasonsSeen[reasons.DeploymentUnavailable],
		"a stalled rollout is reported at once, nothing is held")
}

func TestRolloutHoldHasACap(t *testing.T) {
	m, dep := heldRolloutRig("ReplicaSetUpdated", "True",
		rolloutHoldMax+time.Minute)
	got := heldRolloutFindings(m, dep)
	require.Len(t, got, 1)
	assert.Equal(t, reasons.DeploymentUnavailable, got[0].Reason)
}

func TestNoRolloutNoHold(t *testing.T) {
	m, dep := heldRolloutRig("NewReplicaSetAvailable", "True", 9*time.Minute)
	got := heldRolloutFindings(m, dep)
	require.Len(t, got, 1)
	assert.Equal(t, reasons.DeploymentUnavailable, got[0].Reason)
}

func TestHeldRolloutAsksForAnotherLook(t *testing.T) {
	m, dep := heldRolloutRig("ReplicaSetUpdated", "True", 9*time.Minute)
	eval := evaluate(NewWorkload(5*time.Minute), m, t0, dep, nil)
	assert.Equal(t, rolloutHoldMax-9*time.Minute, eval.RecheckAfter)
}

func TestStatefulSetRollingUpdateIsHeldWhileItMoves(t *testing.T) {
	m := newTestModel()
	sts := newID(kube.KindStatefulSet, "shop", "db")
	attrs := map[string]inventory.Value{
		kube.AttrReplicas:        inventory.Number(3),
		kube.AttrReadyReplicas:   inventory.Number(2),
		kube.AttrRevision:        inventory.Text("db-2"),
		kube.AttrCurrentRevision: inventory.Text("db-1"),
		kube.AttrUpdatedReplicas: inventory.Number(1),
	}
	put(m, sts, t0.Add(-7*time.Minute), attrs)
	got := evaluate(NewWorkload(5*time.Minute), m, t0, sts, nil).Findings
	assert.Empty(t, got)

	// Ten minutes without an updated pod is a stuck rollout.
	later := t0.Add(DefaultRolloutStuck + time.Minute)
	got = evaluate(NewWorkload(5*time.Minute), m, later, sts, nil).Findings
	assert.NotEmpty(t, got)
}

// A young pod of a rolling workload may fail its readiness probe while
// it starts; the same failure after the start budget is reported.
func TestReadinessProbeFailuresDuringARolloutWaitForTheStartBudget(
	t *testing.T,
) {
	for _, tc := range []struct {
		name string
		age  time.Duration
		want bool
	}{
		{"inside the start budget", time.Minute, false},
		{"past the start budget", 6 * time.Minute, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newBaselineRig(map[string]inventory.Value{
				kube.AttrReady: inventory.Bool(false),
			})
			attrs := conditionAttrs(map[string]inventory.Value{},
				"Progressing", "True", "ReplicaSetUpdated", t0)
			put(r.m, r.dep, t0, attrs)
			put(r.m, newID(kube.KindReplicaSet, "shop", "api-1"), t0,
				map[string]inventory.Value{
					kube.AttrCreated: inventory.Time(t0.Add(-tc.age)),
				})
			created := t0.Add(-tc.age)
			put(r.m, r.pod, created, map[string]inventory.Value{
				kube.AttrCreated: inventory.Time(created),
			})
			noteEntity(r.m, r.container, "Unhealthy",
				"Readiness probe failed: connection refused", 5,
				t0.Add(-30*time.Second))
			put(r.m, r.container, t0.Add(-tc.age),
				map[string]inventory.Value{
					kube.AttrState:    inventory.Text("running"),
					kube.AttrReady:    inventory.Bool(false),
					kube.AttrRestarts: inventory.Number(0),
				})
			link(r.m, r.container, inventory.PartOf, r.pod)
			got := evaluate(Container{}, r.m, t0, r.container, nil).Findings
			hasProbe := false
			for _, f := range got {
				hasProbe = hasProbe ||
					f.Reason == reasons.ReadinessProbeFailed
			}
			assert.Equal(t, tc.want, hasProbe)
		})
	}
}
