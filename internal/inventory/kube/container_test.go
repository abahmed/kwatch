package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestContainerID(t *testing.T) {
	id := kube.ContainerID(testNamespace, "mypod", "app")
	assert.Equal(t, inventory.Kind("container"), id.Kind)
	assert.Equal(t, testNamespace, id.Namespace)
	assert.Equal(t, "mypod/app", id.Name)
}

func TestContainerDescriptions(t *testing.T) {
	tt := []struct {
		name    string
		pod     *corev1.Pod
		checkFn func(*testing.T, []kube.Description)
	}{
		{
			name: "single_container",
			pod: func() *corev1.Pod {
				p := pod("p1")
				p.Spec.Containers = []corev1.Container{
					{Name: "app", Image: "app:1.0"},
				}
				return p
			}(),
			checkFn: func(t *testing.T, descs []kube.Description) {
				assert.Len(t, descs, 1)
				desc := descs[0]
				// Container ID should be "p1/app"
				assert.Contains(t, desc.ID.Name, "app")
				// Check image attribute
				img, ok := desc.Attributes["image"]
				assert.True(t, ok)
				assert.Equal(t, "app:1.0", img.AsText())
			},
		},
		{
			name: "init_container",
			pod: func() *corev1.Pod {
				p := pod("p1")
				p.Spec.InitContainers = []corev1.Container{
					{Name: "init", Image: "init:1.0"},
				}
				p.Spec.Containers = []corev1.Container{}
				return p
			}(),
			checkFn: func(t *testing.T, descs []kube.Description) {
				assert.Len(t, descs, 1)
				desc := descs[0]
				init, ok := desc.Attributes["init"]
				assert.True(t, ok)
				b, _ := init.AsBool()
				assert.True(t, b)
			},
		},
		{
			name: "sidecar_container",
			pod: func() *corev1.Pod {
				p := pod("p1")
				restart := corev1.ContainerRestartPolicyAlways
				p.Spec.InitContainers = []corev1.Container{
					{
						Name:          "sidecar",
						Image:         "sidecar:1.0",
						RestartPolicy: &restart,
					},
				}
				p.Spec.Containers = []corev1.Container{}
				return p
			}(),
			checkFn: func(t *testing.T, descs []kube.Description) {
				assert.Len(t, descs, 1)
				desc := descs[0]
				sidecar, ok := desc.Attributes["sidecar"]
				assert.True(t, ok)
				b, _ := sidecar.AsBool()
				assert.True(t, b)
			},
		},
		{
			name: "container_with_resource_limits",
			pod: func() *corev1.Pod {
				p := pod("p1")
				p.Spec.Containers = []corev1.Container{
					{
						Name:  "app",
						Image: "app:1.0",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: mustQuantity("512Mi"),
								corev1.ResourceCPU:    mustQuantity("500m"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: mustQuantity("256Mi"),
								corev1.ResourceCPU:    mustQuantity("100m"),
							},
						},
					},
				}
				return p
			}(),
			checkFn: func(t *testing.T, descs []kube.Description) {
				desc := descs[0]
				// Check memory limits
				memLim, ok := desc.Attributes["memory.limit"]
				assert.True(t, ok)
				num, _ := memLim.AsNumber()
				assert.Equal(t, 512*1024*1024.0, num)
				// Check CPU requests
				cpuReq, ok := desc.Attributes["cpu.request"]
				assert.True(t, ok)
				num, _ = cpuReq.AsNumber()
				assert.Equal(t, 100.0, num) // millis
			},
		},
		{
			name: "container_with_probe_budget",
			pod: func() *corev1.Pod {
				p := pod("p1")
				p.Spec.Containers = []corev1.Container{
					{
						Name:  "app",
						Image: "app:1.0",
						StartupProbe: &corev1.Probe{
							InitialDelaySeconds:           5,
							PeriodSeconds:                 10,
							FailureThreshold:              3,
							SuccessThreshold:              1,
							TimeoutSeconds:                1,
							TerminationGracePeriodSeconds: nil,
						},
					},
				}
				return p
			}(),
			checkFn: func(t *testing.T, descs []kube.Description) {
				desc := descs[0]
				budget, ok := desc.Attributes["probe.budget.seconds"]
				assert.True(t, ok)
				num, _ := budget.AsNumber()
				// 5 + 10*3 = 35
				assert.Equal(t, 35.0, num)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			descs := containerDescriptionsViaSchema(t, tc.pod)
			tc.checkFn(t, descs)
		})
	}
}

// Note: containerDescriptions is not exported, so we need a test
// that calls through the Pod schema
func containerDescriptionsViaSchema(
	t *testing.T, p *corev1.Pod,
) []kube.Description {
	schema := kube.PodSchema{}
	desc, ok := schema.Describe(p)
	assert.True(t, ok)
	return desc.Children
}

func TestContainerStatusAttributes(t *testing.T) {
	tt := []struct {
		name    string
		pod     *corev1.Pod
		checkFn func(*testing.T, kube.Description)
	}{
		{
			name: "running_container",
			pod: func() *corev1.Pod {
				p := pod("p1")
				p.Status.ContainerStatuses = []corev1.ContainerStatus{
					{
						Name:         "app",
						Ready:        true,
						RestartCount: 0,
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{
								StartedAt: metav1.NewTime(
									fixedTime()),
							},
						},
					},
				}
				return p
			}(),
			checkFn: func(t *testing.T, desc kube.Description) {
				state, ok := desc.Attributes["state"]
				assert.True(t, ok)
				assert.Equal(t, "running",
					state.AsText())
				ready, ok := desc.Attributes["ready"]
				assert.True(t, ok)
				b, _ := ready.AsBool()
				assert.True(t, b)
			},
		},
		{
			name: "waiting_container",
			pod: func() *corev1.Pod {
				p := pod("p1")
				p.Status.ContainerStatuses = []corev1.ContainerStatus{
					{
						Name:  "app",
						Ready: false,
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason:  "ImagePullBackOff",
								Message: "Failed to pull image",
							},
						},
					},
				}
				return p
			}(),
			checkFn: func(t *testing.T, desc kube.Description) {
				state, ok := desc.Attributes["state"]
				assert.True(t, ok)
				assert.Equal(t, "waiting",
					state.AsText())
				reason, ok := desc.Attributes["state.reason"]
				assert.True(t, ok)
				assert.Equal(t, "ImagePullBackOff",
					reason.AsText())
			},
		},
		{
			name: "terminated_container",
			pod: func() *corev1.Pod {
				p := pod("p1")
				p.Status.ContainerStatuses = []corev1.ContainerStatus{
					{
						Name: "app",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 1,
								Reason:   "Error",
								FinishedAt: metav1.NewTime(
									fixedTime()),
							},
						},
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 137,
								Reason:   "OOMKilled",
								FinishedAt: metav1.NewTime(
									fixedTime()),
							},
						},
					},
				}
				return p
			}(),
			checkFn: func(t *testing.T, desc kube.Description) {
				state, ok := desc.Attributes["state"]
				assert.True(t, ok)
				assert.Equal(t, "terminated",
					state.AsText())
				exitCode, ok := desc.Attributes["exit.code"]
				assert.True(t, ok)
				num, _ := exitCode.AsNumber()
				assert.Equal(t, 1.0, num)
				lastReason, ok := desc.Attributes["last.reason"]
				assert.True(t, ok)
				assert.Equal(t, "OOMKilled",
					lastReason.AsText())
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			descs := containerDescriptionsViaSchema(
				t, tc.pod)
			assert.Len(t, descs, 1)
			tc.checkFn(t, descs[0])
		})
	}
}
