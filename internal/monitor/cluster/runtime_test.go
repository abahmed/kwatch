package cluster

import (
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func TestDetectNodeLeaseIssueReportsStaleRenewTime(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: nodeLeaseNamespace,
			Name:      "worker-1",
		},
		Spec: coordinationv1.LeaseSpec{
			RenewTime: ptrTime(metav1.NewMicroTime(
				now.Add(-2 * time.Minute),
			)),
		},
	}

	got := DetectNodeLeaseIssue(lease, now, 90)
	if got == nil || got.Reason != constant.ReasonNodeLeaseStale {
		t.Fatalf("expected stale Lease observation, got %#v", got)
	}
}

func TestDetectNodeLeaseIssueIgnoresFreshLease(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Namespace: nodeLeaseNamespace},
		Spec: coordinationv1.LeaseSpec{
			RenewTime: ptrTime(metav1.NewMicroTime(
				now.Add(-time.Minute),
			)),
		},
	}

	if got := DetectNodeLeaseIssue(lease, now, 90); got != nil {
		t.Fatalf("fresh Lease produced observation: %#v", got)
	}
}

func TestLeaseSkipsWhenListerIsUnavailable(t *testing.T) {
	sink := &clusterSinkRecorder{}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)

	if err := runtime.ProcessLease(
		nodeLeaseNamespace+"/worker-1", false,
	); err != nil {
		t.Fatalf("ProcessLease() returned error: %v", err)
	}
	if sink.resolutions != 0 {
		t.Fatalf("unavailable Lease lister resolved %d incidents", sink.resolutions)
	}
}

func TestDeletedLeaseSkipsWhenListerIsUnavailable(t *testing.T) {
	sink := &clusterSinkRecorder{}
	runtime := NewRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)

	if err := runtime.ProcessLease(
		nodeLeaseNamespace+"/worker-1", true,
	); err != nil {
		t.Fatalf("ProcessLease() returned error: %v", err)
	}
	if sink.resolutions != 0 {
		t.Fatalf("unavailable Lease lister resolved %d incidents",
			sink.resolutions)
	}
}

type clusterSinkRecorder struct {
	resolutions int
}

func (s *clusterSinkRecorder) Process(
	*model.Observation,
) (*model.Incident, model.IncidentAction) {
	return nil, model.ActionSkip
}

func (s *clusterSinkRecorder) Resolve(model.ObjectRef, string) {
	s.resolutions++
}

func (*clusterSinkRecorder) ResolveObserved(*model.Observation) {}

func (*clusterSinkRecorder) Reconcile(
	model.ObjectRef, []*model.Observation,
) {
}

func (*clusterSinkRecorder) ReconcileGone(model.ObjectRef) {}

func ptrTime(value metav1.MicroTime) *metav1.MicroTime {
	return &value
}
