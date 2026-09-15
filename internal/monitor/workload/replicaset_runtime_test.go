package workload

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1listers "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

type replicaSetSinkRecorder struct {
	reconciledSubject model.ObjectRef
	reconciled        []*model.Observation
	goneSubject       model.ObjectRef
}

func (r *replicaSetSinkRecorder) Process(
	*model.Observation,
) (*model.Incident, model.IncidentAction) {
	return nil, model.ActionSkip
}

func (r *replicaSetSinkRecorder) Resolve(model.ObjectRef, string) {}

func (r *replicaSetSinkRecorder) ResolveObserved(*model.Observation) {}

func (r *replicaSetSinkRecorder) Reconcile(
	subject model.ObjectRef,
	observations []*model.Observation,
) {
	r.reconciledSubject = subject
	r.reconciled = observations
}

func (r *replicaSetSinkRecorder) ReconcileGone(subject model.ObjectRef) {
	r.goneSubject = subject
}

func TestReplicaSetRuntimeReconcilesDetectedFailure(t *testing.T) {
	indexer := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{},
	)
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "worker"},
		Status: appsv1.ReplicaSetStatus{
			Conditions: []appsv1.ReplicaSetCondition{{
				Type:   appsv1.ReplicaSetReplicaFailure,
				Status: "True",
				Reason: "FailedCreate",
			}},
		},
	}
	if err := indexer.Add(rs); err != nil {
		t.Fatal(err)
	}

	sink := &replicaSetSinkRecorder{}
	runtime := NewReplicaSetRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	runtime.SetLister(appsv1listers.NewReplicaSetLister(indexer))

	if err := runtime.ProcessReplicaSet("default/worker", false); err != nil {
		t.Fatalf("ProcessReplicaSet() returned error: %v", err)
	}
	if sink.reconciledSubject.Kind != "replicaset" {
		t.Fatalf("subject = %+v, want replicaset", sink.reconciledSubject)
	}
	if len(sink.reconciled) != 1 ||
		sink.reconciled[0].Reason != constant.ReasonReplicaSetFailure {
		t.Fatalf(
			"observations = %+v, want %s",
			sink.reconciled,
			constant.ReasonReplicaSetFailure,
		)
	}
}

func TestReplicaSetRuntimeReconcilesDeletedObject(t *testing.T) {
	sink := &replicaSetSinkRecorder{}
	runtime := NewReplicaSetRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, sink, time.Now,
	)
	runtime.SetLister(appsv1listers.NewReplicaSetLister(cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{},
	)))

	if err := runtime.ProcessReplicaSet("default/worker", true); err != nil {
		t.Fatalf("ProcessReplicaSet() returned error: %v", err)
	}
	if sink.goneSubject != (model.ObjectRef{
		Kind: "replicaset", Namespace: "default", Name: "worker",
	}) {
		t.Fatalf("gone subject = %+v", sink.goneSubject)
	}
}

func TestReplicaSetRuntimeRejectsInvalidKey(t *testing.T) {
	runtime := NewReplicaSetRuntimeWithRuntimeConfig(
		config.RuntimeConfig{}, &replicaSetSinkRecorder{}, time.Now,
	)
	if err := runtime.ProcessReplicaSet("worker", false); err == nil {
		t.Fatal("ProcessReplicaSet() returned nil for invalid key")
	}
}
