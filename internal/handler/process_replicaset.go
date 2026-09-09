package handler

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func (h *handler) ProcessReplicaSet(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid replicaset key %q: %w", key, err)
	}
	subject := model.NewObjectRef("replicaset", namespace, name)
	if deleted {
		h.reconcileGone(subject)
		return nil
	}
	rs, err := h.listers.RS.ReplicaSets(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.reconcileGone(subject)
			return nil
		}
		return fmt.Errorf(
			"failed to get replicaset %s/%s from cache: %w",
			namespace, name, err,
		)
	}
	h.reconcile(subject, findings(DetectReplicaSetIssue(rs)))
	return nil
}

// DetectReplicaSetIssue uses the controller's ReplicaFailure condition. The
// condition is important even when a Deployment exists: it carries causes
// such as quota, limit range, node selector and kubelet/finalizer failures.
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
