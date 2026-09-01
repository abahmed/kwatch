package controller

import (
	"bytes"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
)

func (e *errDaemonSetLister) List(
	selector labels.Selector,
) ([]*appsv1.DaemonSet, error) {
	return nil, errors.New("cache not synced")
}

// lockedBuffer is a goroutine-safe writer used to capture klog output.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestBuildSeenSetSurfacesListerErrors(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
			},
		},
	}
	client := fake.NewSimpleClientset(node)
	cfg := &config.Config{
		NodeMonitor: config.NodeMonitor{Enabled: true},
	}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	assert.Eventually(t, func() bool {
		_, err := ctrl.nodeLister.Get("worker-1")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	// Simulate a broken informer cache for DaemonSets: the List error
	// must be surfaced via logging, not silently swallowed.
	ctrl.dsLister = &errDaemonSetLister{}

	var buf lockedBuffer
	klog.LogToStderr(false)
	klog.SetOutput(&buf)
	klog.SetOutputBySeverity("WARNING", &buf)
	klog.SetOutputBySeverity("ERROR", &buf)
	defer func() {
		klog.LogToStderr(true)
		klog.SetOutput(os.Stderr)
		klog.SetOutputBySeverity("WARNING", nil)
		klog.SetOutputBySeverity("ERROR", nil)
	}()

	ctrl.buildSeenSet()

	out := buf.String()
	assert.Contains(t, out, "failed to list daemonsets for baseline seeding")
	assert.Contains(t, out, "cache not synced")

	// A failing lister must not prevent other categories from seeding.
	h.mu.Lock()
	baseline := h.seenBaseline
	h.mu.Unlock()
	expectedKey := correlation.BuildKey("", "worker-1", "MemoryPressure", "")
	assert.Contains(
		t,
		baseline,
		string(expectedKey),
		"other baseline categories must still be seeded",
	)
}

func TestBuildSeenPerPodAndHealthySiblingKeepsBaseline(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "healthy-pod",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "dep", APIVersion: "apps/v1"},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "c",
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "failed-pod",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "dep", APIVersion: "apps/v1"},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodFailed,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "c", State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "Error",
						},
					}}},
			},
		},
	)
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	assert.Eventually(t, func() bool {
		_, err := ctrl.podLister.Pods("default").Get("healthy-pod")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	h.mu.Lock()
	baseline := h.seenBaseline
	h.mu.Unlock()

	// The failed pod's key should be baselined
	key := correlation.BuildKey("default", "dep", "Error", "")
	_, ok := baseline[string(key)]["failed-pod"]
	assert.True(t, ok, "failed pod must be baselined")

	// The healthy pod should NOT be in the baseline
	assert.NotContains(
		t,
		baseline[string(key)],
		"healthy-pod",
		"healthy pod must NOT be baselined",
	)

	// Simulate ClearBaselineForPod for the healthy pod — should NOT affect
	// the failed pod's entry
	h.ClearBaselineForPod("default", "healthy-pod")

	_, ok = baseline[string(key)]["failed-pod"]
	assert.True(
		t,
		ok,
		"ClearBaselineForPod for healthy sibling must not clear failed pod's "+
			"baseline",
	)
}

func TestBuildSeenCrashLoopHighFreq(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cl-pod",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "dep", APIVersion: "apps/v1"},
				},
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "app", RestartCount: 7,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					}}},
			},
		},
	)
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	assert.Eventually(t, func() bool {
		_, err := ctrl.podLister.Pods("default").Get("cl-pod")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	h.mu.Lock()
	baseline := h.seenBaseline
	h.mu.Unlock()

	// Key should be CrashLoopHighFrequency (not CrashLoopBackOff) because
	// restarts > 5
	key := correlation.BuildKey("default", "dep", "CrashLoopHighFrequency", "")
	_, ok := baseline[string(key)]["cl-pod"]
	assert.True(
		t,
		ok,
		"buildSeenSet must use CrashLoopHighFrequency for restarts > 5",
	)
}

func TestBuildSeenRunningWithRestarts(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "restarted-pod",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "dep", APIVersion: "apps/v1"},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "app",
						RestartCount: 3,
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason: "Error",
							},
						},
					},
				},
			},
		},
	)
	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	assert.Eventually(t, func() bool {
		_, err := ctrl.podLister.Pods("default").Get("restarted-pod")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	h.mu.Lock()
	baseline := h.seenBaseline
	h.mu.Unlock()

	// Must use LastTerminationState.Terminated.Reason ("Error"), not skip the
	// pod
	key := correlation.BuildKey("default", "dep", "Error", "")
	_, ok := baseline[string(key)]["restarted-pod"]
	assert.True(
		t,
		ok,
		"Running container with restarts must be baselined using "+
			"LastTerminationState.Reason",
	)
}

func TestBuildSeenSetReportsStartupSummary(t *testing.T) {
	a := assert.New(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "broken-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "broken-rs"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "ImagePullBackOff",
						},
					},
				},
			},
		},
	}

	client := fake.NewSimpleClientset(pod,
		// Need a ReplicaSet for owner resolution
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "broken-rs",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "broken-deploy"},
				},
			},
		},
	)

	cfg := &config.Config{}
	h := &mockHandler{}

	ctrl, cleanup := newTestController(t, client, cfg, h)
	defer cleanup()

	a.Eventually(func() bool {
		_, err := ctrl.podLister.Pods("default").Get("broken-pod")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	a.Eventually(func() bool {
		_, err := ctrl.rsLister.ReplicaSets("default").Get("broken-rs")
		return err == nil
	}, 5*time.Second, 50*time.Millisecond)

	ctrl.buildSeenSet()

	// Must have called ReportStartupSummary with non-empty suppressed counts
	h.mu.Lock()
	summary := h.startupSummary
	h.mu.Unlock()

	a.NotNil(summary, "ReportStartupSummary should have been called")
	a.Greater(
		len(summary),
		0,
		"suppressed map should have entries for broken pods",
	)

	// Verify the suppressed key format: owner/reason
	found := false
	for key, count := range summary {
		if count > 0 {
			found = true
			a.Contains(
				key,
				"/",
				"suppressed key should use owner/reason format",
			)
		}
	}
	a.True(found, "at least one suppressed entry should exist")
}
