package workload

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/constant"
)

func TestDetectDeploymentIssueFindsProgressDeadline(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Status: appsv1.DeploymentStatus{Conditions: []appsv1.DeploymentCondition{
			{
				Type:   appsv1.DeploymentProgressing,
				Status: corev1.ConditionFalse,
				Reason: constant.ReasonProgressDeadlineExceeded,
			},
		}},
	}

	if got := DetectDeploymentIssue(deploy); got == nil {
		t.Fatal("expected a progress deadline finding")
	}
}

func TestDetectDeploymentUnavailableWaitsForObservedGeneration(t *testing.T) {
	replicas := int32(2)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Generation: 2},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			ReadyReplicas:      0,
		},
	}

	if got := DetectDeploymentUnavailable(deploy); got != nil {
		t.Fatal("mid-rollout status must not create availability finding")
	}
}

func TestDetectStatefulSetIssue(t *testing.T) {
	replicas := int32(2)
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default"},
		Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
		Status:     appsv1.StatefulSetStatus{ReadyReplicas: 1},
	}

	if got := DetectStatefulSetIssue(ss); got == nil {
		t.Fatal("expected unavailable StatefulSet finding")
	}
}

func TestDetectDaemonSetIssueFindsUnavailablePods(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 2,
			NumberUnavailable:      1,
		},
	}

	if got := DetectDaemonSetIssue(ds); got == nil {
		t.Fatal("expected unavailable DaemonSet finding")
	}
}

func TestDetectJobExecutionIssueFindsExceededDeadline(t *testing.T) {
	deadline := int64(60)
	started := metav1.NewTime(time.Date(
		2026, 1, 1, 12, 0, 0, 0, time.UTC,
	))
	job := &batchv1.Job{
		Spec: batchv1.JobSpec{ActiveDeadlineSeconds: &deadline},
		Status: batchv1.JobStatus{
			Active:    1,
			StartTime: &started,
		},
	}

	if got := DetectJobExecutionIssue(
		job,
		started.Add(2*time.Minute),
	); got == nil {
		t.Fatal("expected Job deadline finding")
	}
}

func TestJobCompleteRecognizesSuccessfulCompletion(t *testing.T) {
	job := &batchv1.Job{Status: batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{{
			Type:   batchv1.JobComplete,
			Status: corev1.ConditionTrue,
		}},
	}}

	if !JobComplete(job) {
		t.Fatal("expected completed Job")
	}
}

func TestDetectCronJobIssueFindsSuspension(t *testing.T) {
	suspended := true
	cj := &batchv1.CronJob{
		Spec: batchv1.CronJobSpec{Suspend: &suspended},
	}

	if got := DetectCronJobIssue(
		cj,
		time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	); got == nil {
		t.Fatal("expected suspended CronJob finding")
	}
}

func TestDetectPDBIssueFindsBlockingBudget(t *testing.T) {
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Generation: 2},
		Status: policyv1.PodDisruptionBudgetStatus{
			ObservedGeneration: 2,
			DesiredHealthy:     2,
			CurrentHealthy:     1,
		},
	}

	if got := DetectPDBIssue(pdb); got == nil {
		t.Fatal("expected blocking PDB finding")
	}
}

func TestDetectHPAIssuesFindsMaxedOutAutoscaler(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{MaxReplicas: 3},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: 2,
			DesiredReplicas: 3,
		},
	}

	if !HPAAtMax(hpa) || len(DetectHPAIssues(hpa)) != 1 {
		t.Fatal("expected one maxed-out HPA finding")
	}
}

func TestDetectReplicaSetIssueFindsReplicaFailure(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		Status: appsv1.ReplicaSetStatus{
			Conditions: []appsv1.ReplicaSetCondition{{
				Type:    appsv1.ReplicaSetReplicaFailure,
				Status:  corev1.ConditionTrue,
				Reason:  "FailedCreate",
				Message: "quota exceeded",
			}},
		},
	}

	if got := DetectReplicaSetIssue(rs); got == nil {
		t.Fatal("expected ReplicaSet failure finding")
	}
}
