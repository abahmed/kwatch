package pod

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor/pod/enrichment"
	"github.com/abahmed/kwatch/internal/monitor/pod/policy"
)

type evaluatorDetector struct {
	decision policy.Decision
	reason   string
}

func (d evaluatorDetector) Detect(ctx *policy.Context) policy.Decision {
	if ctx.Container != nil && d.reason != "" {
		ctx.Container.Reason = d.reason
		ctx.Container.Msg = "container failed"
	}
	if ctx.Container == nil {
		ctx.PodHasIssues = d.decision == policy.DecisionAlert
		ctx.PodReason = d.reason
	}
	return d.decision
}

type evaluatorSink struct {
	observations []*model.Observation
}

func (s *evaluatorSink) Process(obs *model.Observation) (
	*model.Incident, model.IncidentAction,
) {
	s.observations = append(s.observations, obs)
	return nil, model.ActionCreate
}

func (s *evaluatorSink) Resolve(model.ObjectRef, string) {}

func (s *evaluatorSink) ResolveObserved(*model.Observation) {}

func TestPodEvaluatorContextAndLifecycleHelpers(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api",
	}}
	ctx := &enrichment.Context{Pod: pod}
	if got := contextOwner(ctx); got.Name != "api" || got.Kind != "Pod" {
		t.Fatalf("self owner = %+v", got)
	}
	pod.OwnerReferences = []metav1.OwnerReference{{
		Kind: "Deployment", Name: "api", UID: types.UID("owner"),
	}}
	if got := contextOwner(ctx); got != (model.ObjectRef{}) {
		t.Fatalf("unresolved owner = %+v", got)
	}
	ctx.Owner = &metav1.OwnerReference{Kind: "Deployment", Name: "api"}
	if got := contextOwner(ctx); got.Kind != "Deployment" {
		t.Fatalf("resolved owner = %+v", got)
	}

	deletion := metav1.Now()
	if !podTerminatingOrDisrupted(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deletion},
	}) {
		t.Fatal("deleting pod was not recognized")
	}
	if !podTerminatingOrDisrupted(&corev1.Pod{
		Status: corev1.PodStatus{
			Phase:  corev1.PodFailed,
			Reason: constant.ReasonEvicted,
		},
	}) {
		t.Fatal("evicted pod was not recognized")
	}
	if !podTerminatingOrDisrupted(&corev1.Pod{
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{
			Type: "DisruptionTarget",
		}}},
	}) {
		t.Fatal("disrupted pod was not recognized")
	}
	if podTerminatingOrDisrupted(&corev1.Pod{}) {
		t.Fatal("healthy pod was treated as terminating")
	}
}

func TestPodEvaluatorFormattingHelpers(t *testing.T) {
	when := metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	events := sortedEvents([]*corev1.Event{
		{LastTimestamp: metav1.NewTime(when.Add(time.Minute))},
		nil,
		{LastTimestamp: when},
	})
	if events == nil || len(*events) != 2 ||
		!(*events)[0].LastTimestamp.Equal(&when) {
		t.Fatalf("sorted events = %+v", events)
	}
	if got := sortedEvents(nil); got == nil || len(*got) != 0 {
		t.Fatalf("empty events = %+v", got)
	}

	if got := containerRequestSummary(corev1.Container{Name: "api"}); got != "" {
		t.Fatalf("empty request summary = %q", got)
	}
	requests := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("250m"),
		corev1.ResourceMemory: resource.MustParse("64Mi"),
	}
	got := containerRequestSummary(corev1.Container{
		Name: "api", Resources: corev1.ResourceRequirements{Requests: requests},
	})
	if got != "api requests: cpu=250m mem=64Mi" {
		t.Fatalf("request summary = %q", got)
	}
	status := &corev1.ContainerStatus{LastTerminationState: corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 2},
	}}
	if reason, code := lastTermination(status); reason != "Error" || code != 2 {
		t.Fatalf("termination = %q/%d", reason, code)
	}
	if reason, code := lastTermination(
		&corev1.ContainerStatus{},
	); reason != "" || code != 0 {
		t.Fatalf("empty termination = %q/%d", reason, code)
	}
}

func TestPodEvaluatorHintsAndImagePullBranches(t *testing.T) {
	if got := hintForReasonAndCode(
		constant.ReasonInitContainerError, 1, true,
	); got == "" {
		t.Fatal("init container hint was empty")
	}
	if got := hintForReasonAndCode("", 137, false); got == "" {
		t.Fatal("exit-code hint was empty")
	}
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: "reg"}},
			Containers: []corev1.Container{{
				Name: "api", Image: "registry.example/api:v1",
			}},
		},
	}
	ctx := &enrichment.Context{
		Pod: pod,
		Container: &enrichment.ContainerContext{
			Container: &corev1.ContainerStatus{
				Name: "api", Image: "registry.example/api:v1",
			},
			Msg: "manifest unknown",
		},
	}
	hint, secrets := appendImagePullHint(
		"pull failed", ctx, constant.ReasonErrImagePull,
	)
	if !secrets || hint == "pull failed" {
		t.Fatalf("configured image pull hint = %q, secrets=%v", hint, secrets)
	}
	ctx.Pod.Spec.ImagePullSecrets = nil
	hint, secrets = appendImagePullHint(
		"pull failed", ctx, constant.ReasonErrImagePull,
	)
	if secrets || hint == "pull failed" {
		t.Fatalf("registry image hint = %q, secrets=%v", hint, secrets)
	}
	if hint, _ := appendImagePullHint(
		"pull failed", ctx, "Other",
	); hint != "pull failed" {
		t.Fatalf("unrelated hint = %q", hint)
	}
	if findContainerSpec(pod, "missing") != nil {
		t.Fatal("missing container spec was found")
	}
}

func TestPodEvaluatorPolicyContextHelpers(t *testing.T) {
	clockNow := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	falseCondition := corev1.PodCondition{
		Type: corev1.PodScheduled, Status: corev1.ConditionFalse,
		LastTransitionTime: metav1.NewTime(clockNow.Add(-2 * time.Minute)),
	}
	cfg := &config.Config{
		ScheduleMonitor:           config.ScheduleMonitor{Enabled: true},
		ContainerRestartThreshold: 3,
	}
	evaluator := NewPolicyEvaluatorWithRuntimeConfig(
		config.RuntimeConfigFor(cfg), nil, nil,
		clock.Func(func() time.Time { return clockNow }),
	)
	ctx := &enrichment.Context{Pod: &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.NewTime(clockNow.Add(-time.Minute)),
		},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{falseCondition}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "api", Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
			},
		}}},
	}, PodReason: "Unschedulable"}
	hint, facts := evaluator.podIssueHint(ctx)
	if facts.SchedulingDelay <= 30*time.Second || hint == "" {
		t.Fatalf("unschedulable hint = %q, facts=%+v", hint, facts)
	}
	if got := evaluator.unschedulableDelay(ctx); got != 2*time.Minute {
		t.Fatalf("scheduling delay = %v", got)
	}

	container := &corev1.ContainerStatus{
		Name: "api", RestartCount: 3,
	}
	ctx.Container = &enrichment.ContainerContext{Container: container}
	if !evaluator.highRestartEnabled(ctx) {
		t.Fatal("high restart threshold was not enabled")
	}
	ctx.Pod.Status.Phase = corev1.PodFailed
	ctx.Pod.Status.Reason = constant.ReasonEvicted
	if evaluator.highRestartEnabled(ctx) {
		t.Fatal("evicted pod high restart alert was not suppressed")
	}
}

func TestPolicyEvaluatorEmitsPodAndContainerObservations(t *testing.T) {
	sink := &evaluatorSink{}
	runtime := config.RuntimeConfigFor(&config.Config{})
	evaluator := &PolicyEvaluator{
		runtime: runtime,
		monitor: &Monitor{
			podDetectors: []policy.Detector{
				evaluatorDetector{
					decision: policy.DecisionAlert,
					reason:   "PodFailure",
				},
			},
			containerDetectors: []policy.Detector{
				evaluatorDetector{
					decision: policy.DecisionAlert,
					reason:   constant.ReasonErrImagePull,
				},
			},
		},
		sink: sink,
		now:  time.Now,
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api", UID: "pod-1",
		OwnerReferences: []metav1.OwnerReference{{
			Kind: "Deployment", Name: "api",
		}},
	}, Status: corev1.PodStatus{
		ContainerStatuses: []corev1.ContainerStatus{{
			Name: "api", Image: "registry.example/api:v1",
			State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
				Reason: constant.ReasonErrImagePull,
			}},
		}},
	}}
	ctx := &enrichment.Context{Pod: pod}
	evaluator.EvaluatePod(ctx)
	evaluator.EvaluateContainers(ctx)
	if len(sink.observations) != 2 {
		t.Fatalf("observations = %d, want pod and container", len(sink.observations))
	}
	if sink.observations[0].Container != "." ||
		sink.observations[1].Container != "api" {
		t.Fatalf("observation containers = %+v", sink.observations)
	}
}

func TestPolicyEvaluatorEmitsHighRestartObservation(t *testing.T) {
	sink := &evaluatorSink{}
	cfg := &config.Config{ContainerRestartThreshold: 2}
	evaluator := NewPolicyEvaluatorWithRuntimeConfig(
		config.RuntimeConfigFor(cfg), sink, nil,
		clock.Func(time.Now),
	)
	evaluator.monitor.containerDetectors = []policy.Detector{
		evaluatorDetector{decision: policy.DecisionSuppress},
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Name: "api",
		OwnerReferences: []metav1.OwnerReference{{
			Kind: "Deployment", Name: "api",
		}},
	}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
		Name: "api", RestartCount: 3,
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				Reason: "Error", ExitCode: 1,
			},
		},
	}}}}
	evaluator.EvaluateContainers(&enrichment.Context{
		Pod: pod, Owner: &pod.OwnerReferences[0],
	})
	if len(sink.observations) != 1 ||
		sink.observations[0].Reason != constant.ReasonHighRestartCount {
		t.Fatalf("high restart observations = %+v", sink.observations)
	}
}
