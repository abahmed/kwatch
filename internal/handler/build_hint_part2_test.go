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
