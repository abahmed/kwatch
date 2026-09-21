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

func TestDeploymentPolicyCoversHealthyAndUnavailableStates(t *testing.T) {
	if DetectDeploymentIssue(nil) != nil ||
		DetectDeploymentConditions(nil) != nil {
		t.Fatal("nil deployment produced a finding")
	}
	ready := int32(3)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"},
		Spec:       appsv1.DeploymentSpec{Replicas: &ready},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 1, UpdatedReplicas: 2,
			ObservedGeneration: 2,
		},
	}
	if !DeploymentUnavailable(deploy) || DeploymentDesiredReplicas(deploy) != 3 {
		t.Fatal("deployment availability was not detected")
	}
	if DetectDeploymentUnavailable(deploy) == nil {
		t.Fatal("caught-up unavailable deployment was missed")
	}
	if AvailabilityHint(deploy) == "" {
		t.Fatal("deployment availability hint was empty")
	}
	deploy.Status.Conditions = []appsv1.DeploymentCondition{
		{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionFalse,
			Reason: "Unavailable", Message: "not ready"},
		{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse,
			Reason: constant.ReasonProgressDeadlineExceeded},
	}
	conditions := DetectDeploymentConditions(deploy)
	if len(conditions) != 1 || DetectDeploymentIssue(deploy) == nil {
		t.Fatalf("deployment conditions = %+v", conditions)
	}
	deploy.Spec.Replicas = nil
	deploy.Status.Replicas = 2
	deploy.Status.UnavailableReplicas = 1
	if DeploymentDesiredReplicas(deploy) != 2 || !DeploymentUnavailable(deploy) {
		t.Fatal("observed deployment replica fallback failed")
	}
}

func TestDaemonAndStatefulSetPolicyCoversConditions(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "apps"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3, NumberUnavailable: 1, NumberAvailable: 2,
			Conditions: []appsv1.DaemonSetCondition{{
				Type: "Ready", Status: corev1.ConditionFalse,
				Reason: "NotReady", Message: "waiting",
			}},
		},
	}
	if DetectDaemonSetIssue(ds) == nil ||
		len(DetectDaemonSetConditions(ds)) != 1 ||
		DaemonSetAvailabilityHint(ds) == "" {
		t.Fatal("daemonset policy did not report unavailable pods")
	}
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "apps"},
		Spec:       appsv1.StatefulSetSpec{Replicas: int32Ptr(3)},
		Status: appsv1.StatefulSetStatus{
			Replicas: 3, ReadyReplicas: 1,
			Conditions: []appsv1.StatefulSetCondition{{
				Type: "Ready", Status: corev1.ConditionFalse,
				Reason: "NotReady", Message: "waiting",
			}},
		},
	}
	if DetectStatefulSetIssue(ss) == nil ||
		len(DetectStatefulSetConditions(ss)) != 1 ||
		StatefulSetReplicas(ss) != 3 || !StatefulSetUnavailable(ss) {
		t.Fatal("statefulset policy did not report unavailable pods")
	}
	ss.Spec.Replicas = nil
	if StatefulSetReplicas(ss) != 3 {
		t.Fatal("statefulset status replica fallback failed")
	}
}

func TestCronAndJobPolicyCoversSchedulesAndFailures(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	suspended := true
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: "backup", Namespace: "apps", CreationTimestamp: metav1.NewTime(now),
		},
		Spec: batchv1.CronJobSpec{Suspend: &suspended, Schedule: "*/5 * * * *"},
	}
	if DetectCronJobIssue(cj, now) == nil {
		t.Fatal("suspended cronjob was missed")
	}
	if NextFireAfter("invalid", nil, now, nil) != (time.Time{}) {
		t.Fatal("invalid schedule returned a fire time")
	}
	if !DefaultNextFire(nil, now).Equal(now.Add(24 * time.Hour)) {
		t.Fatal("default cron fire time is wrong")
	}
	start := metav1.NewTime(now.Add(-time.Hour))
	deadline := int64(60)
	backoff := int32(2)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", Namespace: "apps"},
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds: &deadline, BackoffLimit: &backoff,
		},
		Status: batchv1.JobStatus{StartTime: &start, Active: 1, Failed: 2},
	}
	if DetectJobExecutionIssue(job, now) == nil || JobComplete(job) {
		t.Fatal("job execution failure was missed")
	}
	job.Status.Conditions = []batchv1.JobCondition{{
		Type: batchv1.JobFailed, Status: corev1.ConditionTrue,
	}}
	if DetectJobIssue(job) == nil {
		t.Fatal("failed job was missed")
	}
	job.Status.Conditions = []batchv1.JobCondition{{
		Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
	}}
	if !JobComplete(job) {
		t.Fatal("completed job was not recognized")
	}
}

func TestHPAAndPDBPolicyCoversHealthyBranches(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"},
		Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MaxReplicas: 5},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: 5, DesiredReplicas: 5,
			Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{{
				Type:   autoscalingv2.ScalingLimited,
				Status: corev1.ConditionTrue,
				Reason: constant.ReasonTooManyReplicas,
			}},
		},
	}
	if !HPAAtMax(hpa) || len(DetectHPAIssues(hpa)) != 1 {
		t.Fatal("maxed HPA was not detected")
	}
	hpa.Status.Conditions = []autoscalingv2.HorizontalPodAutoscalerCondition{{
		Type:   autoscalingv2.AbleToScale,
		Status: corev1.ConditionFalse,
		Reason: "FailedGetScale",
	}}
	if len(DetectHPAIssues(hpa)) != 1 {
		t.Fatal("HPA scaling failure was not detected")
	}
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "apps"},
		Status: policyv1.PodDisruptionBudgetStatus{
			ObservedGeneration: 1, DesiredHealthy: 3,
			CurrentHealthy: 1, DisruptionsAllowed: 0,
		},
	}
	pdb.Generation = 1
	if !PDBBlocking(pdb) || DetectPDBIssue(pdb) == nil || PDBHint(pdb) == "" {
		t.Fatal("blocking PDB was not detected")
	}
	if PDBBlocking(nil) || DetectPDBIssue(nil) != nil {
		t.Fatal("nil PDB produced a finding")
	}
}

func int32Ptr(value int32) *int32 {
	return &value
}
