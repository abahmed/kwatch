package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/filter"
)

func TestBuildHintOOMKilledNoMemLimit(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{},
						},
					},
				},
			},
		},
		Container: &filter.ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "app",
			},
			Reason:   "OOMKilled",
			ExitCode: 137,
		},
	}
	hint, _ := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "OOMKilled with no memory limit set")
}

// Exit code 137 without an OOMKilled reason is a plain SIGKILL, not an OOM
// kill — the hint must not claim memory pressure.

func TestBuildHintExit137NonOOMKilled(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app"},
				},
			},
		},
		Container: &filter.ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "app",
			},
			Reason:   "Error",
			ExitCode: 137,
		},
	}
	hint, _ := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "Killed (SIGKILL")
	assert.NotContains(t, hint, "OOMKilled")
	assert.NotContains(t, hint, "memory")
}

// A 137 exit from a non-OOM reason must not be recorded as an OOM kill, so it
// can never escalate to the repeating-OOM reason.

func TestBuildHintExit137NonOOMNotTrackedAsOOM(t *testing.T) {
	cfg := &config.Config{
		OomMonitor: config.OomMonitor{
			Enabled:       true,
			Threshold:     3,
			WindowMinutes: 10,
		},
	}
	h := NewHandler(
		fake.NewSimpleClientset(),
		cfg,
		testCorrelator(),
		testAlertMgr,
	)

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app"},
				},
			},
		},
		Container: &filter.ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "app",
			},
			Reason:   "Error",
			ExitCode: 137,
		},
	}

	// Cross the repeating threshold with non-OOM 137 exits.
	for i := 0; i < 5; i++ {
		ctx.Container.Reason = "Error"
		ctx.Container.ExitCode = 137
		hint, _ := h.buildContainerHint(ctx)
		assert.Equal(
			t,
			"Error",
			ctx.Container.Reason,
			"non-OOM 137 exit must not flip reason to OOMRepeating",
		)
		assert.Contains(t, hint, "Killed (SIGKILL")
	}
}

// --- buildContainerHint: CrashLoopBackOff with LivenessProbe ---

func TestBuildHintCrashLoopBackOffLiveness(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/healthz",
									Port: intstr.FromInt(8080),
								},
							},
						},
					},
				},
			},
		},
		Container: &filter.ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "app",
			},
			Reason:   "CrashLoopBackOff",
			ExitCode: 1,
		},
	}
	hint, _ := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "liveness probe")
}

// --- buildContainerHint: ImagePullBackOff with imagePullSecrets ---

func TestBuildHintImagePullBackOffWithSecrets(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "myregistry.io/myimage:latest",
					},
				},
				ImagePullSecrets: []corev1.LocalObjectReference{
					{Name: "my-secret"},
				},
			},
		},
		Container: &filter.ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "app",
			},
			Reason:   "ImagePullBackOff",
			ExitCode: 0,
		},
	}
	hint, facts := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "imagePullSecrets")
	assert.True(
		t,
		facts.PullSecretsSet,
		"the renderer learns about the secrets from the facts, not the prose",
	)
}

// --- buildContainerHint: ImagePullBackOff with well-known error message ---

func TestBuildHintImagePullBackOffRateLimit(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app", Image: "nginx"},
				},
			},
		},
		Container: &filter.ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "app",
			},
			Reason: "ImagePullBackOff",
			Msg:    "toomanyrequests: rate limit exceeded",
		},
	}
	hint, _ := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "rate limit")
}

// --- executeContainersFilters: container with Msg set for buildHint ---

func TestBuildHintOOMKilledWithLimit(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse(
									"256Mi",
								),
							},
						},
					},
				},
			},
		},
		Container: &filter.ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "app",
			},
			Reason:   "OOMKilled",
			ExitCode: 137,
		},
	}
	hint, facts := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "memory limit")
	assert.Equal(t, "256Mi", facts.MemoryLimit)
}

// --- buildContainerHint: CrashLoopBackOff without LivenessProbe ---

func TestBuildHintCrashLoopBackOffNoLiveness(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app"},
				},
			},
		},
		Container: &filter.ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "app",
			},
			Reason:   "CrashLoopBackOff",
			ExitCode: 1,
		},
	}
	hint, _ := h.buildContainerHint(ctx)
	assert.NotEmpty(t, hint)
}

// --- pod issue with signalEvent ---

func TestBuildHintNoSpecFound(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec:       corev1.PodSpec{},
		},
		Container: &filter.ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "nonexistent",
			},
			Reason:   "CrashLoopBackOff",
			ExitCode: 1,
		},
	}
	hint, _ := h.buildContainerHint(ctx)
	assert.NotEmpty(t, hint)
}

// --- Init container error ---

func TestBuildHintInitContainerError(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					{Name: "init", Image: "busybox"},
				},
			},
		},
		Container: &filter.ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "init",
			},
			Reason:   "Error",
			ExitCode: 1,
			IsInit:   true,
		},
	}
	hint, _ := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "init container")
}

// --- Unschedulable with creation delay (no condition transition) ---

func TestBuildHintLivenessProbeFailed(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/healthz",
									Port: intstr.FromInt(8080),
								},
							},
						},
					},
				},
			},
		},
		Container: &filter.ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "app",
			},
			Reason:   "LivenessProbeFailed",
			ExitCode: 0,
		},
	}
	hint, facts := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "liveness")
	assert.Equal(t, "HTTP GET http://app:8080/healthz", facts.ProbeEndpoint)
}

// --- buildContainerHint: OOMRepeating with primed oomTracker ---

func TestBuildHintOOMRepeating(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{
		OomMonitor: config.OomMonitor{
			Enabled:       true,
			Threshold:     2,
			WindowMinutes: 10,
		},
	}, testCorrelator(), testAlertMgr)

	// Prime the tracker with one record so the next call repeats
	h.oomTracker.record("ns1/p1/app")

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse(
									"256Mi",
								),
							},
						},
					},
				},
			},
		},
		Container: &filter.ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "app",
			},
			Reason:   "OOMKilled",
			ExitCode: 137,
		},
	}
	hint, facts := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "OOMKilled")
	assert.Contains(t, hint, "potential memory leak")
	assert.True(t, facts.MemoryLeak)
	assert.Equal(t, 2, facts.OOMCount)
	assert.Equal(t, 10, facts.OOMWindowMin)
	assert.NotEmpty(t, facts.OOMTimeline)
}

// --- Unschedulable delay fallback (delay <= 0) ---
