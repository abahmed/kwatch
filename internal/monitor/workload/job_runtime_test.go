package workload

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	batchv1listers "k8s.io/client-go/listers/batch/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func TestJobRuntimeReconcilesDetectedFailure(t *testing.T) {
	indexer := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{},
	)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "backup"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{{
				Type:   batchv1.JobFailed,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	if err := indexer.Add(job); err != nil {
		t.Fatal(err)
	}

	sink := &replicaSetSinkRecorder{}
	runtime := NewJobRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	runtime.configureLister(batchv1listers.NewJobLister(indexer))

	if err := runtime.ProcessJob("default/backup", false); err != nil {
		t.Fatalf("ProcessJob() returned error: %v", err)
	}
	if sink.reconciledSubject.Kind != "job" {
		t.Fatalf("subject = %+v, want job", sink.reconciledSubject)
	}
	if len(sink.reconciled) != 1 ||
		sink.reconciled[0].Reason != constant.ReasonJobFailed {
		t.Fatalf("observations = %+v, want JobFailed", sink.reconciled)
	}
}

func TestJobRuntimeReconcilesCompletedJobAsGone(t *testing.T) {
	indexer := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{},
	)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "backup"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{{
				Type:   batchv1.JobComplete,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	if err := indexer.Add(job); err != nil {
		t.Fatal(err)
	}

	sink := &replicaSetSinkRecorder{}
	runtime := NewJobRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	runtime.configureLister(batchv1listers.NewJobLister(indexer))

	if err := runtime.ProcessJob("default/backup", false); err != nil {
		t.Fatalf("ProcessJob() returned error: %v", err)
	}
	if sink.goneSubject != (model.ObjectRef{
		Kind: "job", Namespace: "default", Name: "backup",
	}) {
		t.Fatalf("gone subject = %+v", sink.goneSubject)
	}
}
