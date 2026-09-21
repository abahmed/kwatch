package pvc

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestPVCStatusHelpersCoverFailureKinds(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	deleting := metav1.NewTime(now.Add(-11 * time.Minute))
	if !volumeStuckTerminating(&deleting, []string{"cleanup"}, now) {
		t.Fatal("stuck volume was not detected")
	}
	if volumeStuckTerminating(nil, nil, now) {
		t.Fatal("volume without deletion was stuck")
	}
	if !pvcStatusFailure(corev1.ClaimPending) ||
		!pvcStatusFailure(corev1.ClaimLost) ||
		pvcStatusFailure(corev1.ClaimBound) {
		t.Fatal("PVC phase failure classification changed")
	}
	if !pvStatusFailure(corev1.VolumeReleased) ||
		!pvStatusFailure(corev1.VolumeFailed) ||
		pvStatusFailure(corev1.VolumeAvailable) {
		t.Fatal("PV phase failure classification changed")
	}
	if got := pvcFailureCondition(corev1.PersistentVolumeClaimStatus{
		AllocatedResourceStatuses: map[corev1.ResourceName]corev1.ClaimResourceStatus{
			"storage": corev1.PersistentVolumeClaimControllerResizeInfeasible,
		},
	}); got == "" {
		t.Fatal("resize failure condition was not reported")
	}
	if got := pvcFailureCondition(corev1.PersistentVolumeClaimStatus{
		ModifyVolumeStatus: &corev1.ModifyVolumeStatus{
			Status: corev1.PersistentVolumeClaimModifyVolumeInfeasible,
		},
	}); got == "" {
		t.Fatal("modify failure condition was not reported")
	}
	if got := joinStatusDetails("reason", "message"); got != "reason: message" {
		t.Fatalf("joined status = %q", got)
	}
	if joinStatusDetails("", "message") != "message" ||
		joinStatusDetails("reason", "") != "reason" {
		t.Fatal("empty status details were not handled")
	}
}

func TestPVCSourceScopeAndObservationOrdering(t *testing.T) {
	monitor := newTestPvcMonitor(nil, nil)
	if err := monitor.ConfigureSources(Sources{
		Allowed: []string{"apps"}, Forbidden: []string{"blocked"},
		WatchAll: false, NamespaceAllowed: func(namespace string) bool {
			return namespace != "filtered"
		},
	}); err != nil {
		t.Fatal(err)
	}
	if !monitor.namespaceAllowed("blocked") ||
		monitor.namespaceAllowed("filtered") ||
		!monitor.namespaceAllowed("apps") {
		t.Fatal("namespace scope classification changed")
	}
	if err := monitor.ConfigureSources(Sources{}); err == nil {
		t.Fatal("second source configuration succeeded")
	}
	monitor = newTestPvcMonitor(nil, nil)
	monitor.started = true
	if err := monitor.ConfigureSources(Sources{}); err == nil {
		t.Fatal("late source configuration succeeded")
	}
	observations := []*model.Observation{
		{Subject: model.ObjectRef{Namespace: "z", Name: "b"}},
		{Subject: model.ObjectRef{Namespace: "a", Name: "c"}},
	}
	sortObservations(observations)
	if observations[0].Subject.Namespace != "a" {
		t.Fatalf("observations were not sorted: %+v", observations)
	}
}

func TestGetNodeUsageParsesAndFiltersSummary(t *testing.T) {
	monitor := newTestPvcMonitor(nil, nil)
	monitor.getNodeUsageFn = func(
		context.Context, string, map[string]string,
	) ([]*PvcUsage, error) {
		return []*PvcUsage{{Name: "claim"}}, nil
	}
	usage, err := monitor.getNodeUsage(context.Background(), "node-a",
		map[string]string{"apps/claim": "pv"})
	if err != nil || len(usage) != 1 || usage[0].Name != "claim" {
		t.Fatalf("injected usage = %#v, %v", usage, err)
	}
	if got := NewPvcMonitorWithRuntimeAndClock(
		fake.NewSimpleClientset(), config.RuntimeConfig{}, nil, nil,
		fixedClock{now: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)},
	); got == nil {
		t.Fatal("runtime PVC constructor returned nil")
	}
}

func TestCheckVolumeStatusReportsPVCAndPVFailures(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	deleting := metav1.NewTime(now.Add(-11 * time.Minute))
	client := fake.NewSimpleClientset(
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pending", Namespace: "apps",
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimPending,
			},
		},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: "stuck", Namespace: "apps",
				DeletionTimestamp: &deleting,
				Finalizers:        []string{"cleanup"},
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase: corev1.ClaimBound,
			},
		},
		&corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "pv", Finalizers: []string{
				"cleanup",
			}, DeletionTimestamp: &deleting},
			Status: corev1.PersistentVolumeStatus{
				Phase: corev1.VolumeFailed, Reason: "failed", Message: "broken",
			},
		},
	)
	monitor := newTestPvcMonitorWithState(
		client, &config.PvcMonitor{Enabled: true},
		newTestIncidentEngine(), nil,
	)
	monitor.now = func() time.Time { return now }
	monitor.checkVolumeStatus(context.Background())
}

func TestPVCStartStopsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	monitor := newTestPvcMonitor(nil, nil)
	monitor.config.Enabled = true
	monitor.Start(ctx)
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }
