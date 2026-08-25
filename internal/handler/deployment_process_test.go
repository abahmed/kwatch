package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestProcessDeploymentObjectUnavailable(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		RolloutMonitor: config.RolloutMonitor{SustainedMinutes: 0}, // fire immediately
	}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	// Seed deployment into informer
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-deploy",
			Namespace:  "default",
			Generation: 2,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration:  2,
			Replicas:            3,
			ReadyReplicas:       2,
			UnavailableReplicas: 1,
			UpdatedReplicas:     2,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionTrue,
					Reason: "ReplicaSetUpdated",
				},
			},
		},
	}
	f := informers.NewSharedInformerFactory(client, 0)
	f.Apps().V1().Deployments().Informer().GetIndexer().Add(deploy)
	h.SetDeploymentLister(f.Apps().V1().Deployments().Lister())

	// Process via string key (like the controller does)
	err := h.ProcessDeployment("default/my-deploy", false)
	assert.NoError(t, err)

	// DeploymentUnavailable should fire as a signal event
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessDeploymentObjectUnavailableSustained(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		RolloutMonitor: config.RolloutMonitor{SustainedMinutes: 10}, // requires 10 min sustained
	}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-deploy",
			Namespace:  "default",
			Generation: 2,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration:  2,
			Replicas:            3,
			ReadyReplicas:       2,
			UnavailableReplicas: 1,
			UpdatedReplicas:     2,
		},
	}
	f := informers.NewSharedInformerFactory(client, 0)
	f.Apps().V1().Deployments().Informer().GetIndexer().Add(deploy)
	h.SetDeploymentLister(f.Apps().V1().Deployments().Lister())

	// First call: should NOT fire (not yet sustained)
	err := h.ProcessDeployment("default/my-deploy", false)
	assert.NoError(t, err)
	assert.Equal(t, 0, e.ActiveCount(), "should not fire before sustained window")

	// Second call with same deployment: still not sustained without time travel
	err = h.ProcessDeployment("default/my-deploy", false)
	assert.NoError(t, err)
	assert.Equal(t, 0, e.ActiveCount(), "still not sustained")
}

func TestProcessDeploymentObjectHealthy(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		RolloutMonitor: config.RolloutMonitor{SustainedMinutes: 2},
	}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	// Healthy deployment: all replicas ready/available
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "healthy-deploy",
			Namespace:  "default",
			Generation: 2,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration:  2,
			Replicas:            3,
			ReadyReplicas:       3,
			AvailableReplicas:   3,
			UnavailableReplicas: 0,
			UpdatedReplicas:     3,
		},
	}
	f := informers.NewSharedInformerFactory(client, 0)
	f.Apps().V1().Deployments().Informer().GetIndexer().Add(deploy)
	h.SetDeploymentLister(f.Apps().V1().Deployments().Lister())

	err := h.ProcessDeployment("default/healthy-deploy", false)
	assert.NoError(t, err)
	assert.Equal(t, 0, e.ActiveCount(), "healthy deployment should not create incidents")
}

func TestProcessDeploymentObjectProgressDeadlineExceeded(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stuck-deploy",
			Namespace: "default",
		},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionFalse,
					Reason: "ProgressDeadlineExceeded",
				},
			},
		},
	}
	f := informers.NewSharedInformerFactory(client, 0)
	f.Apps().V1().Deployments().Informer().GetIndexer().Add(deploy)
	h.SetDeploymentLister(f.Apps().V1().Deployments().Lister())

	err := h.ProcessDeployment("default/stuck-deploy", false)
	assert.NoError(t, err)
	assert.Equal(t, 1, e.ActiveCount(), "ProgressDeadlineExceeded should create an incident")
}

func TestProcessDeploymentObjectDeleted(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	// Process a deletion
	err := h.ProcessDeployment("default/missing-deploy", true)
	assert.NoError(t, err)
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessDeploymentObjectDirectDeleted(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-deploy", Namespace: "default"},
	}
	assert.NoError(t, h.ProcessDeploymentObject(deploy, true))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessDeploymentObjectNotYetObserved(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		RolloutMonitor: config.RolloutMonitor{SustainedMinutes: 0},
	}
	e := testCorrelator()
	h := NewHandler(client, cfg, e, testAlertMgr)

	// Generation mismatch: ObservedGeneration < Generation
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "new-deploy",
			Namespace:  "default",
			Generation: 3,
		},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration:  2,
			Replicas:            3,
			UnavailableReplicas: 3,
		},
	}
	f := informers.NewSharedInformerFactory(client, 0)
	f.Apps().V1().Deployments().Informer().GetIndexer().Add(deploy)
	h.SetDeploymentLister(f.Apps().V1().Deployments().Lister())

	err := h.ProcessDeployment("default/new-deploy", false)
	assert.NoError(t, err)
	assert.Equal(t, 0, e.ActiveCount(), "not yet observed deployment should not fire")
}

func TestProcessDeploymentInvalidKey(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.Error(t, h.ProcessDeployment("a/b/c", false))
}

func TestProcessDeploymentNotFound(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetDeploymentLister(f.Apps().V1().Deployments().Lister())

	assert.NoError(t, h.ProcessDeployment("default/missing", false))
	assert.Equal(t, 0, e.ActiveCount())
}
