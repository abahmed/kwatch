package handler

import (
	"regexp"
	"testing"

	"github.com/abahmed/kwatch/alertmanager"
	"github.com/abahmed/kwatch/config"
	"github.com/abahmed/kwatch/storage"
	"github.com/abahmed/kwatch/storage/memory"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewHandler(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)
	assert.NotNil(t, h)
}

func TestProcessPodNilObject(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)
	h.ProcessPod("ADDED", nil)
}

func TestProcessPodInvalidType(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)
	h.ProcessPod("ADDED", &corev1.Node{})
}

func TestProcessPodDeleted(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	mem.AddPodContainer("default", "test-pod", "test-container", &storage.ContainerState{})

	h.ProcessPod("DELETED", pod)
}

func TestProcessNodeNilObject(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)
	h.ProcessNode("ADDED", nil)
}

func TestProcessNodeInvalidType(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)
	h.ProcessNode("ADDED", &corev1.Pod{})
}

func TestProcessNodeDeleted(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	mem.AddNode("test-node")
	h.ProcessNode("DELETED", node)
}

func TestProcessNodeNotReadyNoAlert(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		IgnoreNodeReasons:  []string{"KubeletNotReady"},
		IgnoreNodeMessages: []string{"specific message"},
	}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionFalse,
					Reason:  "KubeletNotReady",
					Message: "kubelet is not ready",
				},
			},
		},
	}

	h.ProcessNode("ADDED", node)
}

func TestProcessNodeReadyRecovery(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	mem.AddNode("test-node")

	h := NewHandler(client, cfg, mem, alertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
					Reason: "KubeletReady",
				},
			},
		},
	}

	h.ProcessNode("MODIFIED", node)
}

func TestProcessNodeNotReadyAlert(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionFalse,
					Reason:  "KubeletNotReady",
					Message: "kubelet is not ready",
				},
			},
		},
	}

	h.ProcessNode("ADDED", node)
}

func TestProcessNodeNotReadyWithIgnoredMessage(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		IgnoreNodeMessages: []string{"draining"},
	}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionFalse,
					Reason:  "NodeNotReady",
					Message: "node is draining for maintenance",
				},
			},
		},
	}

	h.ProcessNode("ADDED", node)
}

func TestProcessNodeAlreadyKnownNotReady(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	mem.AddNode("test-node")

	h := NewHandler(client, cfg, mem, alertMgr)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionFalse,
					Reason:  "KubeletNotReady",
					Message: "kubelet is not ready",
				},
			},
		},
	}

	h.ProcessNode("MODIFIED", node)
}

func TestProcessPodWithPodIssues(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)

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

	h.ProcessPod("ADDED", pod)
}

func TestProcessPodWithContainersIssues(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		MaxRecentLogLines: 10,
	}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)

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

	h.ProcessPod("ADDED", pod)
}

func TestProcessPodIgnoredNamespace(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ForbiddenNamespaces: []string{"kube-system"},
	}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)

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

	h.ProcessPod("ADDED", pod)
}

func TestProcessPodIgnoredPodName(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		IgnorePodNamePatterns: []*regexp.Regexp{regexp.MustCompile("^test-.*")},
	}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)

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

	h.ProcessPod("ADDED", pod)
}

func TestProcessPodIgnoredContainerName(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		MaxRecentLogLines:    10,
		IgnoreContainerNames: []string{"test-container"},
	}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)

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

	h.ProcessPod("ADDED", pod)
}

func TestProcessPodSucceededPhase(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
		},
	}

	h.ProcessPod("ADDED", pod)
}

func TestProcessPodCompletedStatus(t *testing.T) {
	client := fake.NewSimpleClientset()
	cfg := &config.Config{}
	mem := memory.NewMemory()
	alertMgr := &alertmanager.AlertManager{}

	h := NewHandler(client, cfg, mem, alertMgr)

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

	h.ProcessPod("ADDED", pod)
}
