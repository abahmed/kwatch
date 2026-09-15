package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func TestReconcileResolvesReasonsThatDisappear(t *testing.T) {
	e := NewEngine(Config{Window: 10})
	subject := model.NewObjectRef("service", "default", "api")
	obs := observe.ObjectNamed(
		"service", "default", "api", "NoReadyEndpoints",
	)

	e.Reconcile(subject, []*model.Observation{obs})
	key := ObservationKey(obs)
	active := e.state[key]
	require.NotNil(t, active)
	assert.Equal(t, model.StateActive, active.State)

	e.Reconcile(subject, nil)
	resolved := e.state[key]
	require.NotNil(t, resolved)
	assert.Equal(t, model.StateResolved, resolved.State)
}

func TestReconcileGoneClosesWorkloadAndPodSubjects(t *testing.T) {
	e := NewEngine(Config{Window: 10})
	workload := model.NewObjectRef("deployment", "default", "api")
	deploymentObs := observe.ObjectNamed(
		"deployment", "default", "api", "RolloutStuck",
	)
	pod := observe.PodOwnedBy(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default", Name: "api",
			},
		},
		"container", "CrashLoopBackOff", workload,
	)

	e.Process(pod)
	e.Reconcile(workload, []*model.Observation{deploymentObs})
	deploymentKey := ObservationKey(deploymentObs)
	podKey := ObservationKey(pod)
	require.Equal(t, model.StateActive, e.state[deploymentKey].State)
	require.Equal(t, model.StateActive, e.state[podKey].State)

	e.ReconcileGone(workload)
	assert.Equal(t, model.StateResolved, e.state[deploymentKey].State)
	assert.Equal(t, model.StateResolved, e.state[podKey].State)
}
