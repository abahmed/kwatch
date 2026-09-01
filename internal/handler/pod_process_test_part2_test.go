package handler

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/filter"
)

func TestProcessPodWithCrashLoopBackOffLivenessProbe(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "liveness-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "node1",
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
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 3,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "Error",
							ExitCode: 1,
						},
					},
				},
			},
		},
	}

	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessPodUnschedulableWithEventLister(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ScheduleMonitor: config.ScheduleMonitor{Enabled: true},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)
	hh := h
	hh.now = func() time.Time { return time.Now().Add(2 * time.Minute) }

	// Set event lister to exercise that branch in executePodFilters
	f := informers.NewSharedInformerFactory(client, 0)
	h.listers.Event = f.Core().V1().Events().Lister()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "unscheduled",
			Namespace:         "default",
			CreationTimestamp: metav1.Now(),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("500m"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodScheduled,
					Status:             corev1.ConditionFalse,
					Reason:             "Unschedulable",
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}

	err := h.ProcessPodObject(context.Background(), pod, false)
	assert.NoError(t, err)
	snap := e.Snapshot()
	assert.NotEmpty(t, snap)
}

func TestProcessPodNilObject(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessPodObject(context.Background(), nil, false))
}

func TestProcessPodDeleted(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, true))
}

func TestProcessPodWithPodIssues(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "no nodes available",
				},
			},
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

func TestProcessPodWithContainersIssues(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		MaxRecentLogLines: 10,
	}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "test-container",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "container is crashing",
						},
					},
				},
			},
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

func TestProcessPodIgnoredNamespace(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ForbiddenNamespaces: []string{"kube-system"},
	}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "kube-system",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "no nodes available",
				},
			},
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

func TestProcessPodIgnoredPodName(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	cfg.Suppression = config.SuppressionIndex{
		PodNamePatterns: []*regexp.Regexp{regexp.MustCompile("^test-.*")},
	}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "no nodes available",
				},
			},
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

func TestProcessPodIgnoredContainerName(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	cfg.Suppression = config.SuppressionIndex{
		ContainerNames: []string{"test-container"},
	}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "test-container",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "container is crashing",
						},
					},
				},
			},
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

func TestProcessPodSucceededPhase(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

func TestProcessPodCompletedStatus(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}

	h := NewHandler(client, cfg, testCorrelator(), testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionFalse,
					Reason: "PodCompleted",
				},
			},
		},
	}

	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

func TestEmitHighRestartAlertNoOwner(t *testing.T) {
	h := NewHandler(
		fake.NewSimpleClientset(),
		&config.Config{},
		testCorrelator(),
		testAlertMgr,
	)

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
		},
	}
	h.emitHighRestartAlert(ctx, &corev1.ContainerStatus{
		Name:         "c1",
		RestartCount: 5,
	})
}
