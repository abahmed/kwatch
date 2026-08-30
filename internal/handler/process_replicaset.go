package handler

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
)

func (h *handler) ProcessReplicaSet(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid replicaset key %q: %w", key, err)
	}
	owner := namespace + "/" + name
	if deleted {
		h.correlator.ResolveByResource("replicaset", owner)
		return nil
	}
	rs, err := h.listers.RS.ReplicaSets(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("replicaset", owner)
			return nil
		}
		return fmt.Errorf("failed to get replicaset %s from cache: %w", owner, err)
	}
	if sig := DetectReplicaSetIssue(rs); sig != nil {
		h.signalEvent(sig)
	} else {
		h.correlator.ResolveByResource("replicaset", owner)
	}
	return nil
}

// DetectReplicaSetIssue uses the controller's ReplicaFailure condition. The
// condition is important even when a Deployment exists: it carries causes
// such as quota, limit range, node selector and kubelet/finalizer failures.
func DetectReplicaSetIssue(rs *appsv1.ReplicaSet) *event.Signal {
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
		return &event.Signal{
			Resource: "replicaset", Namespace: rs.Namespace,
			PodName: rs.Name, Owner: rs.Namespace + "/" + rs.Name,
			Reason: constant.ReasonReplicaSetFailure,
			Labels: rs.Labels, Hint: hint,
		}
	}
	return nil
}
