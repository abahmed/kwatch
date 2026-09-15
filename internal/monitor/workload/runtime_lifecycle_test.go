package workload

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func TestDeploymentRuntimeReconcilesRolloutFailure(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "api",
		},
		Status: appsv1.DeploymentStatus{Conditions: []appsv1.DeploymentCondition{{
			Type:   appsv1.DeploymentProgressing,
			Status: "False",
			Reason: constant.ReasonProgressDeadlineExceeded,
		}}},
	}
	indexer := workloadIndexer(t, deploy)
	sink := &replicaSetSinkRecorder{}
	runtime := NewDeploymentRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	runtime.SetLister(appsv1lister.NewDeploymentLister(indexer))

	if err := runtime.ProcessDeployment("default/api", false); err != nil {
		t.Fatalf("ProcessDeployment() returned error: %v", err)
	}
	assertRuntimeReason(t, sink, constant.ReasonProgressDeadlineExceeded)
}

func TestWorkloadRuntimesSkipWhenListerIsUnavailable(t *testing.T) {
	runtime := NewDeploymentRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, &replicaSetSinkRecorder{}, time.Now,
	)
	if err := runtime.ProcessDeployment("default/api", false); err != nil {
		t.Fatalf("ProcessDeployment() returned error: %v", err)
	}
}

func TestWorkloadRuntimesSkipDeletedWhenListerIsUnavailable(t *testing.T) {
	tests := []struct {
		name string
		call func(*replicaSetSinkRecorder) error
	}{
		{
			name: "deployment",
			call: func(sink *replicaSetSinkRecorder) error {
				runtime := NewDeploymentRuntimeWithRuntimeConfig(
					config.RuntimeConfig{}, sink, time.Now,
				)
				return runtime.ProcessDeployment("default/api", true)
			},
		},
		{
			name: "replicaset",
			call: func(sink *replicaSetSinkRecorder) error {
				runtime := NewReplicaSetRuntimeWithRuntimeConfig(
					config.RuntimeConfig{}, sink, time.Now,
				)
				return runtime.ProcessReplicaSet("default/api", true)
			},
		},
		{
			name: "statefulset",
			call: func(sink *replicaSetSinkRecorder) error {
				runtime := NewStatefulSetRuntimeWithRuntimeConfig(
					config.RuntimeConfig{}, sink, time.Now,
				)
				return runtime.ProcessStatefulSet("default/api", true)
			},
		},
		{
			name: "daemonset",
			call: func(sink *replicaSetSinkRecorder) error {
				runtime := NewDaemonSetRuntimeWithRuntimeConfig(
					config.RuntimeConfig{}, sink, nil, time.Now,
				)
				return runtime.ProcessDaemonSet("default/api", true)
			},
		},
		{
			name: "job",
			call: func(sink *replicaSetSinkRecorder) error {
				runtime := NewJobRuntimeWithRuntimeConfig(
					config.RuntimeConfig{}, sink, time.Now,
				)
				return runtime.ProcessJob("default/api", true)
			},
		},
		{
			name: "cronjob",
			call: func(sink *replicaSetSinkRecorder) error {
				runtime := NewCronJobRuntimeWithRuntimeConfig(
					config.RuntimeConfig{}, sink, time.Now,
				)
				return runtime.ProcessCronJob("default/api", true)
			},
		},
		{
			name: "hpa",
			call: func(sink *replicaSetSinkRecorder) error {
				runtime := NewHPARuntimeWithRuntimeConfig(
					config.RuntimeConfig{}, sink, time.Now,
				)
				return runtime.ProcessHorizontalPodAutoscaler(
					"default/api", true,
				)
			},
		},
		{
			name: "pdb",
			call: func(sink *replicaSetSinkRecorder) error {
				runtime := NewPDBRuntimeWithRuntimeConfig(
					config.RuntimeConfig{}, sink, time.Now,
				)
				return runtime.ProcessPdb("default/api", true)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink := &replicaSetSinkRecorder{}
			if err := test.call(sink); err != nil {
				t.Fatalf("process returned error: %v", err)
			}
			if sink.goneSubject != (model.ObjectRef{}) {
				t.Fatalf("unavailable lister resolved %+v", sink.goneSubject)
			}
		})
	}
}

func TestWorkloadRuntimeIgnoresLateSourceConfiguration(t *testing.T) {
	sink := &replicaSetSinkRecorder{}
	runtime := NewDeploymentRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	if err := runtime.ProcessDeployment("default/api", false); err != nil {
		t.Fatalf("initial ProcessDeployment() returned error: %v", err)
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"},
		Status: appsv1.DeploymentStatus{Conditions: []appsv1.DeploymentCondition{{
			Type:   appsv1.DeploymentProgressing,
			Status: "False",
			Reason: constant.ReasonProgressDeadlineExceeded,
		}}},
	}
	runtime.SetLister(appsv1lister.NewDeploymentLister(
		workloadIndexer(t, deploy),
	))
	if err := runtime.ProcessDeployment("default/api", false); err != nil {
		t.Fatalf("late ProcessDeployment() returned error: %v", err)
	}
	if len(sink.reconciled) != 0 {
		t.Fatalf("late source configuration changed runtime: %+v",
			sink.reconciled)
	}
}

func TestDaemonSetRuntimeReconcilesCondition(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "agent",
		},
		Status: appsv1.DaemonSetStatus{Conditions: []appsv1.DaemonSetCondition{{
			Type:   appsv1.DaemonSetConditionType("Progressing"),
			Status: "False",
			Reason: "ImagePullBackOff",
		}}},
	}
	indexer := workloadIndexer(t, ds)
	sink := &replicaSetSinkRecorder{}
	runtime := NewDaemonSetRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, nil, time.Now,
	)
	runtime.SetLister(appsv1lister.NewDaemonSetLister(indexer))

	if err := runtime.ProcessDaemonSet("default/agent", false); err != nil {
		t.Fatalf("ProcessDaemonSet() returned error: %v", err)
	}
	assertRuntimeReason(t, sink, constant.ReasonDaemonSetCondition)
}

func TestStatefulSetRuntimeReconcilesUnavailableReplicas(t *testing.T) {
	replicas := int32(2)
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "database",
		},
		Spec:   appsv1.StatefulSetSpec{Replicas: &replicas},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: 1},
	}
	indexer := workloadIndexer(t, ss)
	sink := &replicaSetSinkRecorder{}
	runtime := NewStatefulSetRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	runtime.SetLister(appsv1lister.NewStatefulSetLister(indexer))

	if err := runtime.ProcessStatefulSet("default/database", false); err != nil {
		t.Fatalf("ProcessStatefulSet() returned error: %v", err)
	}
	assertRuntimeReason(t, sink, constant.ReasonStsUnavailable)
}

func TestCronJobRuntimeReconcilesSuspension(t *testing.T) {
	suspended := true
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "backup",
		},
		Spec: batchv1.CronJobSpec{Suspend: &suspended},
	}
	indexer := workloadIndexer(t, cj)
	sink := &replicaSetSinkRecorder{}
	runtime := NewCronJobRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	runtime.SetLister(batchv1lister.NewCronJobLister(indexer))
	runtime.support.now = func() time.Time {
		return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	}

	if err := runtime.ProcessCronJob("default/backup", false); err != nil {
		t.Fatalf("ProcessCronJob() returned error: %v", err)
	}
	assertRuntimeReason(t, sink, constant.ReasonCronJobSuspended)
}

func TestHPARuntimeReconcilesMaxedOutAutoscaler(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "api",
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{MaxReplicas: 3},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: 2,
			DesiredReplicas: 3,
		},
	}
	indexer := workloadIndexer(t, hpa)
	sink := &replicaSetSinkRecorder{}
	runtime := NewHPARuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	runtime.SetLister(
		autoscalingv2lister.NewHorizontalPodAutoscalerLister(indexer),
	)

	if err := runtime.ProcessHorizontalPodAutoscaler(
		"default/api",
		false,
	); err != nil {
		t.Fatalf(
			"ProcessHorizontalPodAutoscaler() returned error: %v",
			err,
		)
	}
	assertRuntimeReason(t, sink, constant.ReasonHPAMaxedOut)
}

func TestPDBRuntimeReconcilesBlockingBudget(t *testing.T) {
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "default",
			Name:       "api",
			Generation: 1,
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			ObservedGeneration: 1,
			DesiredHealthy:     2,
			CurrentHealthy:     1,
		},
	}
	indexer := workloadIndexer(t, pdb)
	sink := &replicaSetSinkRecorder{}
	runtime := NewPDBRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	runtime.SetLister(
		policyv1lister.NewPodDisruptionBudgetLister(indexer),
	)

	if err := runtime.ProcessPdb("default/api", false); err != nil {
		t.Fatalf("ProcessPdb() returned error: %v", err)
	}
	assertRuntimeReason(t, sink, constant.ReasonPdbViolation)
}

func workloadIndexer(t *testing.T, object interface{}) cache.Indexer {
	t.Helper()
	indexer := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{},
	)
	if err := indexer.Add(object); err != nil {
		t.Fatal(err)
	}
	return indexer
}

func assertRuntimeReason(
	t *testing.T,
	sink *replicaSetSinkRecorder,
	want string,
) {
	t.Helper()
	if len(sink.reconciled) != 1 || sink.reconciled[0].Reason != want {
		t.Fatalf(
			"observations = %+v, want one observation with reason %s",
			sink.reconciled,
			want,
		)
	}
}
