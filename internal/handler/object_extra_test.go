package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/filter"
)

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
