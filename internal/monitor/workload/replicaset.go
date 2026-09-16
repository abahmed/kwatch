package workload

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// DetectReplicaSetIssue returns the controller's ReplicaFailure finding.
func DetectReplicaSetIssue(rs *appsv1.ReplicaSet) *model.Observation {
	if rs == nil {
		return nil
	}
	for _, condition := range rs.Status.Conditions {
		if condition.Type != appsv1.ReplicaSetReplicaFailure ||
			condition.Status != "True" {
			continue
		}
		hint := condition.Reason
		if condition.Message != "" {
			hint += ": " + condition.Message
		}
		return observe.Object(
			"replicaset", rs, constant.ReasonReplicaSetFailure,
		).WithHint(hint)
	}
	return nil
}

// ReplicaSetProcessor is the controller-facing contract for ReplicaSet
// processing. It belongs to the workload family and not to controller
// infrastructure.
type ReplicaSetProcessor interface {
	ProcessReplicaSet(string, bool) error
}

// ReplicaSetRuntime owns ReplicaSet lookup and lifecycle reconciliation while
// the detector above remains a pure workload policy function.
type ReplicaSetRuntime struct {
	support runtimeSupport
	lister  appsv1lister.ReplicaSetLister
}

// NewReplicaSetRuntimeWithRuntimeConfig constructs ReplicaSet monitoring from
// the immutable runtime snapshot.
func NewReplicaSetRuntimeWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink LifecycleSink,
	now func() time.Time,
) *ReplicaSetRuntime {
	return &ReplicaSetRuntime{
		support: newRuntimeSupportAt(runtime, sink, now),
	}
}

// configureLister supplies the informer-backed ReplicaSet cache after
// controller
// construction.
func (r *ReplicaSetRuntime) configureLister(
	lister appsv1lister.ReplicaSetLister,
) {
	r.support.configureSource(func() { r.lister = lister })
}

// ProcessReplicaSet reconciles one ReplicaSet queue key.
func (r *ReplicaSetRuntime) ProcessReplicaSet(
	key string,
	deleted bool,
) error {
	return processKey(
		&r.support, key, "replicaset", deleted,
		func(namespace, _ string) error {
			if namespace == "" {
				return fmt.Errorf(
					"invalid replicaset key %q: namespace is empty", key,
				)
			}
			return nil
		},
		func() (appsv1lister.ReplicaSetLister, bool) {
			lister := r.support.sourceSnapshot(
				func() appsv1lister.ReplicaSetLister { return r.lister },
			)
			return lister, lister != nil
		},
		func(
			lister appsv1lister.ReplicaSetLister,
			namespace, name string,
		) (*appsv1.ReplicaSet, error) {
			return lister.ReplicaSets(namespace).Get(name)
		},
		func(subject model.ObjectRef) {
			r.support.reconcileGone(subject)
		},
		func(subject model.ObjectRef, rs *appsv1.ReplicaSet) error {
			if r.support.maintenance(rs.Annotations) {
				r.support.reconcileGone(subject)
				return nil
			}
			r.support.reconcile(
				subject, observations(DetectReplicaSetIssue(rs)),
			)
			return nil
		},
	)
}
