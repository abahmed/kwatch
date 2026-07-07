package handler

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	clienttesting "k8s.io/client-go/testing"
)

// --- ProcessPod success path ---

func TestProcessPodListerSuccess(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			},
		},
	}
	f.Core().V1().Pods().Informer().GetIndexer().Add(pod)
	h := NewHandler(client, &config.Config{}, e, testAlertMgr)
	h.SetPodLister(f.Core().V1().Pods().Lister())
	assert.NoError(t, h.ProcessPod(context.Background(), "ns1/p1", false))
}

// --- ProcessServiceObject endpoint slice lister error ---

func TestProcessServiceObjectEndpointSliceListerError(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetEndpointSliceLister(&errorEndpointSliceLister{f.Discovery().V1().EndpointSlices().Lister()})
	h.SetServiceLister(f.Core().V1().Services().Lister())

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc1", Namespace: "ns1"},
	}
	assert.Error(t, h.ProcessServiceObject(svc, false))
}

// --- ProcessIngressObject nil serviceLister ---

func TestProcessIngressObjectNilServiceLister(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "ing1", Namespace: "ns1"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path: "/",
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: "nonexistent-svc",
									Port: networkingv1.ServiceBackendPort{Number: 80},
								},
							},
						}},
					},
				},
			}},
		},
	}
	assert.NoError(t, h.ProcessIngressObject(ing, false))
}

// --- DetectControlPlanePodIssue: CrashLoopBackOff + LastTerminationState ---

func TestDetectControlPlanePodIssueCrashLoopBackOffLastTerm(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "cp-pod", Namespace: "kube-system", Labels: map[string]string{"component": "kube-apiserver"}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "apiserver",
					RestartCount: 3,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "OOMKilled",
							ExitCode: 137,
						},
					},
				},
			},
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.NotNil(t, sig)
	assert.Equal(t, "ControlPlaneComponentFailure", sig.Reason)
}

// --- DetectCronJobIssue: NextFireAfter returns zero ---

func TestDetectCronJobIssueInvalidSchedule(t *testing.T) {
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj1", Namespace: "ns1", CreationTimestamp: metav1.NewTime(time.Now().Add(-48 * time.Hour))},
		Spec:       batchv1.CronJobSpec{Schedule: "invalid"},
	}
	sig := DetectCronJobIssue(cj)
	assert.NotNil(t, sig)
	assert.Equal(t, "CronJobNotScheduled", sig.Reason)
}

// --- ProcessCronJobObject: sustained suspension ---

func TestProcessCronJobObjectSustainedSuspension(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{
		CronJobMonitor: config.CronJobMonitor{SustainedMinutes: 10},
	}, e, testAlertMgr)
	suspend := true
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj1", Namespace: "ns1"},
		Spec:       batchv1.CronJobSpec{Suspend: &suspend},
	}
	assert.NoError(t, h.ProcessCronJobObject(cj, false))
	assert.Equal(t, 0, e.ActiveCount(), "sustained window should suppress alert")
}

// --- executePodFilters: event lister with events ---

func TestProcessPodEventListerWithEvents(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "unschedulable-pod",
		},
		Type:   "Warning",
		Reason: "FailedScheduling",
	}
	f.Core().V1().Events().Informer().GetIndexer().Add(ev)
	h.SetEventLister(f.Core().V1().Events().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available",
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executePodFilters: event lister error ---

func TestProcessPodEventListerError(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	h := NewHandler(client, &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	h.SetEventLister(&errorEventLister{f.Core().V1().Events().Lister()})

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available",
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executePodFilters: k8s.GetPodEvents error ---

func TestProcessPodGetEventsError(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "events", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, assert.AnError
	})
	h := NewHandler(client, &config.Config{}, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available",
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executePodFilters: PodEventsFilter enricher skip (deleting event) ---

func TestProcessPodEventsDeletingPod(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "deleting-pod",
		},
		Type:    "Warning",
		Reason:  "FailedScheduling",
		Message: "deleting pod for some reason",
	}
	f.Core().V1().Events().Informer().GetIndexer().Add(ev)
	h.SetEventLister(f.Core().V1().Events().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "deleting-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available",
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 0, e.ActiveCount(), "deleting event should suppress alert")
}

// --- executePodFilters: owner resolved ---

func TestProcessPodOwnerResolved(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rs", Namespace: "default", UID: "rs-uid"},
	}
	f.Apps().V1().ReplicaSets().Informer().GetIndexer().Add(rs)
	h.SetReplicaLister(f.Apps().V1().ReplicaSets().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rs-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "my-rs", UID: "rs-uid"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available",
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executePodFilters: Unschedulable with ScheduleMonitor ---

func TestProcessPodUnschedulableDelay(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		MaxRecentLogLines: 10,
		ScheduleMonitor:   config.ScheduleMonitor{Enabled: true},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default", CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute))},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodScheduled,
					Status:             corev1.ConditionFalse,
					Reason:             "Unschedulable",
					Message:            "0/1 nodes available",
					LastTransitionTime: metav1.NewTime(time.Now().Add(-3 * time.Minute)),
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executeContainersFilters: event lister error ---

func TestProcessContainerEventListerError(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	h := NewHandler(client, &config.Config{MaxRecentLogLines: 10}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	h.SetEventLister(&errorEventLister{f.Core().V1().Events().Lister()})

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "broken-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executeContainersFilters: event lister with matching events ---

func TestProcessContainerEventListerWithEvents(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	h := NewHandler(client, &config.Config{MaxRecentLogLines: 10}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "broken-pod",
		},
		Type:   "Warning",
		Reason: "BackOff",
	}
	f.Core().V1().Events().Informer().GetIndexer().Add(ev)
	h.SetEventLister(f.Core().V1().Events().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "broken-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executeContainersFilters: k8s.GetPodEvents error ---

func TestProcessContainerGetEventsError(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "events", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, assert.AnError
	})
	h := NewHandler(client, &config.Config{MaxRecentLogLines: 10}, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "broken-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executeContainersFilters: ContainerKillingFilter enricher skip ---

func TestProcessContainerKillingFilterSkip(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		MaxRecentLogLines:            10,
		IgnoreFailedGracefulShutdown: true,
	}
	h := NewHandler(client, cfg, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "killed-pod",
		},
		Type:    "Warning",
		Reason:  "Killing",
		Message: "Stopping container app",
	}
	f.Core().V1().Events().Informer().GetIndexer().Add(ev)
	h.SetEventLister(f.Core().V1().Events().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "killed-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 1,
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "Error",
							ExitCode: 137,
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 0, e.ActiveCount(), "killing filter should skip container")
}

// --- executeContainersFilters: owner resolved ---

func TestProcessContainerOwnerResolved(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rs", Namespace: "default", UID: "rs-uid"},
	}
	f.Apps().V1().ReplicaSets().Informer().GetIndexer().Add(rs)
	h.SetReplicaLister(f.Apps().V1().ReplicaSets().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "broken-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "my-rs", UID: "rs-uid"},
			},
		},
		Spec: corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- buildContainerHint: OOMKilled with spec but no memory limit ---

func TestBuildHintOOMKilledNoMemLimit(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr).(*handler)

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
	hint := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "OOMKilled with no memory limit set")
}

// --- buildContainerHint: CrashLoopBackOff with LivenessProbe ---

func TestBuildHintCrashLoopBackOffLiveness(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr).(*handler)

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
	hint := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "liveness probe")
}

// --- buildContainerHint: ImagePullBackOff with imagePullSecrets ---

func TestBuildHintImagePullBackOffWithSecrets(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr).(*handler)

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
	hint := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "imagePullSecrets")
}

// --- buildContainerHint: ImagePullBackOff with well-known error message ---

func TestBuildHintImagePullBackOffRateLimit(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr).(*handler)

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
	hint := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "rate limit")
}

// --- executeContainersFilters: container with Msg set for buildHint ---

func TestProcessContainerWithMsg(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	h := NewHandler(client, &config.Config{MaxRecentLogLines: 10}, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "broken-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 3,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "ErrImagePull",
							Message: "Back-off pulling image",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- container issue with container message populated (Msg path in buildsignalEvent) ---
func TestProcessContainerSignalEvent(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "broken-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
}

// --- CronJob issue: invalid schedule through ProcessCronJob ---

func TestProcessCronJobInvalidSchedule(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj1", Namespace: "ns1", CreationTimestamp: metav1.NewTime(time.Now().Add(-48 * time.Hour))},
		Spec:       batchv1.CronJobSpec{Schedule: "invalid"},
	}
	f.Batch().V1().CronJobs().Informer().GetIndexer().Add(cj)
	h := NewHandler(client, &config.Config{}, e, testAlertMgr)
	h.SetCronJobLister(f.Batch().V1().CronJobs().Lister())
	assert.NoError(t, h.ProcessCronJob("ns1/cj1", false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- Control plane pod with Running container (continue) ---

func TestDetectControlPlanePodRunningContinue(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "cp-pod", Namespace: "kube-system", Labels: map[string]string{"component": "kube-apiserver"}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "apiserver",
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.Nil(t, sig, "healthy running control plane should return nil")
}

// --- Unschedulable with resource requests ---

func TestProcessPodUnschedulableWithResources(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ScheduleMonitor: config.ScheduleMonitor{Enabled: true},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default", CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute))},
		Spec: corev1.PodSpec{
			NodeName: "node1",
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodScheduled,
					Status:             corev1.ConditionFalse,
					Reason:             "Unschedulable",
					Message:            "0/1 nodes available",
					LastTransitionTime: metav1.NewTime(time.Now().Add(-3 * time.Minute)),
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- OOMKilled with memory limit set (buildContainerHint line 195) ---

func TestBuildHintOOMKilledWithLimit(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr).(*handler)

	ctx := &filter.Context{
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "ns1"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("256Mi"),
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
	hint := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "memory limit")
}

// --- buildContainerHint: CrashLoopBackOff without LivenessProbe ---

func TestBuildHintCrashLoopBackOffNoLiveness(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr).(*handler)

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
	hint := h.buildContainerHint(ctx)
	assert.NotEmpty(t, hint)
}

// --- pod issue with signalEvent ---

func TestProcessPodSignalEvent(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	h := NewHandler(client, &config.Config{}, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available",
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- ProcessIngressObject with serviceLister set (no service found safe path) ---
func TestProcessIngressObjectWithServiceLister(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	f := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	h.SetServiceLister(f.Core().V1().Services().Lister())

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "ing1", Namespace: "ns1"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: "example.com",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path: "/",
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: "nonexistent-svc",
									Port: networkingv1.ServiceBackendPort{Number: 80},
								},
							},
						}},
					},
				},
			}},
		},
	}
	assert.NoError(t, h.ProcessIngressObject(ing, false))
}

// --- CronJob with valid schedule and no last schedule (not scheduled) ---
func TestProcessCronJobNotScheduledPath(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj1", Namespace: "ns1", CreationTimestamp: metav1.NewTime(time.Now().Add(-48 * time.Hour))},
		Spec:       batchv1.CronJobSpec{Schedule: "*/5 * * * *"},
	}
	f.Batch().V1().CronJobs().Informer().GetIndexer().Add(cj)
	h := NewHandler(client, &config.Config{}, e, testAlertMgr)
	h.SetCronJobLister(f.Batch().V1().CronJobs().Lister())
	assert.NoError(t, h.ProcessCronJob("ns1/cj1", false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- Container no spec found for buildContainerHint (spec != nil false in probe section) ---
func TestBuildHintNoSpecFound(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr).(*handler)

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
	hint := h.buildContainerHint(ctx)
	assert.NotEmpty(t, hint)
}

// --- Init container error ---
func TestBuildHintInitContainerError(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr).(*handler)

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
	hint := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "init container")
}

// --- Unschedulable with creation delay (no condition transition) ---
func TestProcessPodUnschedulableCreationDelay(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ScheduleMonitor: config.ScheduleMonitor{Enabled: true},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default", CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute))},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available",
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- emitHighRestartAlert early return when owner resolves to empty (RS owner, no rsLister) ---
func TestEmitHighRestartAlertOwnerEmpty(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ContainerRestartThreshold: 1,
		MaxRecentLogLines:         10,
	}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restart-pod",
			Namespace: "ns1",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "my-rs", UID: "rs-uid"},
			},
		},
		Spec: corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "OOMKilled",
							ExitCode: 137,
						},
					},
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 0, e.ActiveCount(), "empty owner should suppress high restart alert")
}

// --- executePodFilters: StatusContinue from PendingPodFilter ---
func TestProcessPodStatusContinue(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		PendingPodMonitor: config.PendingPodMonitor{Enabled: true, Threshold: 60},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available",
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executePodFilters: !ctx.PodHasIssues return after enrichment ---
func TestProcessPodNoIssuesAfterEnrich(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	h := NewHandler(client, &config.Config{}, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	ev := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "unschedulable-pod",
		},
		Type:    "Warning",
		Reason:  "FailedScheduling",
		Message: "deleting pod for maintenance",
	}
	f.Core().V1().Events().Informer().GetIndexer().Add(ev)
	h.SetEventLister(f.Core().V1().Events().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available",
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 0, e.ActiveCount(), "deleting event should clear PodHasIssues")
}

// --- executePodFilters: sort.Slice in event lister path with 2 events ---
func TestProcessPodEventListerMultiEvents(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	now := metav1.Now()
	t1 := metav1.NewTime(now.Time.Add(-10 * time.Second))
	t2 := metav1.NewTime(now.Time.Add(-5 * time.Second))
	ev1 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "unschedulable-pod",
		},
		Type:          "Warning",
		Reason:        "FailedScheduling",
		LastTimestamp: t1,
	}
	ev2 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev2", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "unschedulable-pod",
		},
		Type:          "Warning",
		Reason:        "FailedScheduling",
		LastTimestamp: t2,
	}
	f.Core().V1().Events().Informer().GetIndexer().Add(ev1)
	f.Core().V1().Events().Informer().GetIndexer().Add(ev2)
	h.SetEventLister(f.Core().V1().Events().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "0/1 nodes available",
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- executeContainersFilters: sort.Slice in event lister path with 2 events ---
func TestProcessContainerEventListerMultiEvents(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{MaxRecentLogLines: 10}
	h := NewHandler(client, cfg, e, testAlertMgr)

	f := informers.NewSharedInformerFactory(client, 0)
	now := metav1.Now()
	t1 := metav1.NewTime(now.Time.Add(-10 * time.Second))
	t2 := metav1.NewTime(now.Time.Add(-5 * time.Second))
	ev1 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "broken-pod",
		},
		Type:          "Warning",
		Reason:        "BackOff",
		LastTimestamp: t1,
	}
	ev2 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ev2", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Pod",
			Name: "broken-pod",
		},
		Type:          "Warning",
		Reason:        "BackOff",
		LastTimestamp: t2,
	}
	f.Core().V1().Events().Informer().GetIndexer().Add(ev1)
	f.Core().V1().Events().Informer().GetIndexer().Add(ev2)
	h.SetEventLister(f.Core().V1().Events().Lister())

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "broken-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- buildContainerHint: LivenessProbeFailed with spec ---
func TestBuildHintLivenessProbeFailed(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr).(*handler)

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
	hint := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "liveness")
}

// --- buildContainerHint: OOMRepeating with primed oomTracker ---
func TestBuildHintOOMRepeating(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{
		OomMonitor: config.OomMonitor{
			Enabled:       true,
			Threshold:     2,
			WindowMinutes: 10,
		},
	}, testCorrelator(), testAlertMgr).(*handler)

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
								corev1.ResourceMemory: resource.MustParse("256Mi"),
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
	hint := h.buildContainerHint(ctx)
	assert.Contains(t, hint, "OOMKilled")
	assert.Contains(t, hint, "potential memory leak")
}

// --- Unschedulable delay fallback (delay <= 0) ---
func TestProcessPodUnschedulableDelayFallback(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		ScheduleMonitor: config.ScheduleMonitor{Enabled: true},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)
	hh := h.(*handler)
	hh.now = func() time.Time { return time.Now().Add(-1 * time.Minute) }

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unschedulable-pod", Namespace: "default", CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute))},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodScheduled,
					Status:             corev1.ConditionFalse,
					Reason:             "Unschedulable",
					Message:            "0/1 nodes available",
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}
	assert.NoError(t, hh.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, e.ActiveCount())
}

// --- DetectControlPlanePodIssue: CrashLoopBackOff with LastTerminationState ---
func TestDetectControlPlanePodIssueCrashLoopBackOffWithLastTerm(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "cp-pod", Namespace: "kube-system", Labels: map[string]string{"component": "kube-apiserver"}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "apiserver",
					RestartCount: 3,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: "CrashLoopBackOff",
						},
					},
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason:   "OOMKilled",
							ExitCode: 137,
							Message:  "memory limit exceeded",
						},
					},
				},
			},
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.NotNil(t, sig)
	assert.Equal(t, "ControlPlaneComponentFailure", sig.Reason)
}

// --- DetectControlPlanePodIssue: Waiting reason fallback (ImagePullBackOff) ---
func TestDetectControlPlanePodIssueWaitingFallback(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "cp-pod", Namespace: "kube-system", Labels: map[string]string{"component": "kube-scheduler"}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "scheduler",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "ImagePullBackOff",
							Message: "Back-off pulling image",
						},
					},
				},
			},
		},
	}
	sig := DetectControlPlanePodIssue(pod)
	assert.NotNil(t, sig)
	assert.Equal(t, "ControlPlaneComponentFailure", sig.Reason)
}

// mock enricher that clears PodHasIssues without skipping
type clearPodIssuesEnricher struct{}

func (clearPodIssuesEnricher) Enrich(ctx *filter.Context) bool {
	ctx.PodHasIssues = false
	return false
}

// --- executePodFilters: PodHasIssues cleared by enricher after detection ---
func TestProcessPodNoIssuesAfterPodEnricher(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	cfg := &config.Config{
		PendingPodMonitor: config.PendingPodMonitor{Enabled: true, Threshold: 600},
	}
	h := NewHandler(client, cfg, e, testAlertMgr)
	hh := h.(*handler)
	hh.podEnrichers = []filter.Enricher{clearPodIssuesEnricher{}}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pending-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: ""},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodScheduled,
					Status: corev1.ConditionUnknown,
				},
			},
		},
	}
	assert.NoError(t, hh.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 0, e.ActiveCount())
}
