package workload

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

type observedSink struct {
	replicaSetSinkRecorder
	processed []*model.Observation
}

func (s *observedSink) Process(
	obs *model.Observation,
) (*model.Incident, model.IncidentAction) {
	s.processed = append(s.processed, obs)
	return nil, model.ActionSkip
}

func TestDeploymentRolloutConditionsWaitForSustainWindow(t *testing.T) {
	replicas := int32(1)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			Replicas: 1, UnavailableReplicas: 1,
			Conditions: []appsv1.DeploymentCondition{{
				Type:   appsv1.DeploymentAvailable,
				Status: corev1.ConditionFalse,
				Reason: "MinimumReplicasUnavailable",
			}},
		},
	}
	sink := &observedSink{}
	runtime := NewDeploymentRuntimeWithRuntimeConfig(
		config.CompileRuntimeConfig(config.DefaultConfig()), sink, time.Now,
	)
	runtime.configureLister(
		appsv1lister.NewDeploymentLister(workloadIndexer(t, deploy)),
	)
	if err := runtime.ProcessDeployment("default/api", false); err != nil {
		t.Fatal(err)
	}
	if len(sink.processed) != 0 {
		t.Fatalf("rollout condition alerted inside the sustain window: %s",
			sink.processed[0].Reason)
	}
}
