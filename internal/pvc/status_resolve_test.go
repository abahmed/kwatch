package pvc

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

type resolveRecorder struct {
	resolved []string
}

func (r *resolveRecorder) Process(
	*model.Observation,
) (*model.Incident, model.IncidentAction) {
	return nil, model.ActionSkip
}

func (r *resolveRecorder) Resolve(_ model.ObjectRef, reason string) {
	r.resolved = append(r.resolved, reason)
}

func (r *resolveRecorder) ResolveObserved(*model.Observation) {}

func TestHealthyPVCStatusResolvesOnlyStatusIncident(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "default"},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
		},
	})
	sink := &resolveRecorder{}
	m := newTestPvcMonitorWithState(
		client, &config.PvcMonitor{Enabled: true}, sink, nil,
	)
	m.watchAll = true
	m.checkVolumeStatus(context.Background())
	if len(sink.resolved) != 1 ||
		sink.resolved[0] != constant.ReasonPersistentVolumeClaim {
		t.Fatalf("resolved reasons = %q, want only the status reason",
			sink.resolved)
	}
}
