package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
)

func TestBuildSeenSeedsDaemonSetBaselineWithEmptyKey(t *testing.T) {
	a := assert.New(t)

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ds",
			Namespace: "default",
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			NumberUnavailable:      1,
			NumberAvailable:        2,
		},
	}
	client := fake.NewSimpleClientset(ds)
	cfg := &config.Config{
		DaemonSetMonitor: config.DaemonSetMonitor{Enabled: true},
	}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	a.Eventually(func() bool {
		_, err := ctrl.dsLister.DaemonSets("default").Get("test-ds")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	h.mu.Lock()
	baseline := h.seenBaseline
	h.mu.Unlock()

	key := correlation.BuildKey(
		"default",
		"default/test-ds",
		"DaemonSetUnavailable",
		"",
	)
	a.Contains(
		baseline,
		string(key),
		"buildSeenSet must seed DaemonSet issues into baseline",
	)

	_, hasEmpty := baseline[string(key)][""]
	a.True(
		hasEmpty,
		"controller resource baseline must map under empty pod key",
	)
}

func TestBuildSeenSeedsDeploymentUnavailableBaseline(t *testing.T) {
	a := assert.New(t)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-dep",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:            3,
			UnavailableReplicas: 1,
			ObservedGeneration:  1,
		},
	}
	client := fake.NewSimpleClientset(deploy)
	cfg := &config.Config{
		RolloutMonitor: config.RolloutMonitor{Enabled: true},
	}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	a.Eventually(func() bool {
		_, err := ctrl.deployLister.Deployments("default").Get("test-dep")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	h.mu.Lock()
	baseline := h.seenBaseline
	h.mu.Unlock()

	key := correlation.BuildKey(
		"default",
		"default/test-dep",
		"DeploymentUnavailable",
		"",
	)
	a.Contains(
		baseline,
		string(key),
		"buildSeenSet must seed DeploymentUnavailable issues into baseline",
	)

	_, hasEmpty := baseline[string(key)][""]
	a.True(
		hasEmpty,
		"deployment resource baseline must map under empty pod key",
	)
}

func TestBuildSeenSetReportsEmptySummaryOnNoBrokenPods(t *testing.T) {
	a := assert.New(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "healthy-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}

	client := fake.NewSimpleClientset(pod)
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	a.Eventually(func() bool {
		_, err := ctrl.podLister.Pods("default").Get("healthy-pod")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	h.mu.Lock()
	summary := h.startupSummary
	h.mu.Unlock()

	// Must be empty or nil (no broken pods to suppress)
	a.Empty(summary, "no broken pods means empty startup summary")
}

// A key whose sync fails every time must eventually be dropped. Before the cap
// it circulated with backoff for the life of the process.
func TestProcessNextItemGivesUpAfterMaxRetries(t *testing.T) {
	p := newResourcePipeline("pod", "pods")
	// The production limiter backs off exponentially into the minutes; the
	// test only cares about the count, so requeue with no delay.
	p.queue = workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.NewTypedItemExponentialFailureRateLimiter[string](0, 0),
		workqueue.TypedRateLimitingQueueConfig[string]{Name: "test"},
	)
	calls := 0
	p.syncFn = func(context.Context, string) error {
		calls++
		return errors.New("always fails")
	}
	p.queue.Add("default/doomed")

	// Drive the queue until the key is no longer requeued. Rate-limited
	// re-adds land after a delay, so poll with a bound rather than spin.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if p.queue.Len() == 0 && p.queue.NumRequeues("default/doomed") == 0 {
			break
		}
		if p.queue.Len() > 0 {
			require.True(t, p.processNextItem(context.Background()))
			continue
		}
		time.Sleep(5 * time.Millisecond)
	}

	assert.Equal(
		t,
		maxSyncRetries+1,
		calls,
		"one initial attempt plus maxSyncRetries requeues",
	)
	assert.Equal(
		t,
		0,
		p.queue.NumRequeues("default/doomed"),
		"the key must be forgotten",
	)
	assert.Equal(t, 0, p.queue.Len(), "nothing left in the queue")
}

// One informer that can never sync — a missing RBAC rule, an absent API group
// — must not park kwatch forever. The error has to name the resource.
