package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
)

// DetectStatefulSetIssue returns a Signal if the StatefulSet has unavailable
// pods that would trigger an alert. Used for baseline seeding at startup.
func DetectStatefulSetIssue(ss *appsv1.StatefulSet) *event.Signal {
	if ss == nil {
		return nil
	}
	if statefulSetUnavailable(ss) {
		return &event.Signal{
			Resource:  "statefulset",
			Reason:    constant.ReasonStsUnavailable,
			Namespace: ss.Namespace,
			Owner:     ss.Namespace + "/" + ss.Name,
			Labels:    ss.Labels,
			Hint:      stsAvailabilityHint(ss),
		}
	}
	return nil
}

func DetectStatefulSetConditions(ss *appsv1.StatefulSet) []*event.Signal {
	if ss == nil {
		return nil
	}
	owner := ss.Namespace + "/" + ss.Name
	var out []*event.Signal
	for _, condition := range ss.Status.Conditions {
		if condition.Status == corev1.ConditionTrue {
			continue
		}
		hint := string(condition.Type) + ": " + condition.Reason
		if condition.Message != "" {
			hint += " — " + condition.Message
		}
		out = append(out, &event.Signal{Resource: "statefulset", Namespace: ss.Namespace,
			PodName: ss.Name, Owner: owner, Reason: constant.ReasonStatefulSetCondition,
			Labels: ss.Labels, Hint: hint})
	}
	return out
}

func stsAvailabilityHint(ss *appsv1.StatefulSet) string {
	desired := statefulSetReplicas(ss)
	notReady := desired - ss.Status.ReadyReplicas
	return fmt.Sprintf(
		"%d/%d pods not ready (ready: %d) — check PVC, pod status, or rollout "+
			"progress",
		notReady,
		desired,
		ss.Status.ReadyReplicas,
	)
}

func (h *handler) ProcessStatefulSet(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid statefulset key %q: %w", key, err)
	}

	if deleted {
		h.clearFirstUnavailableSts(namespace + "/" + name)
		h.correlator.ResolveByResource("statefulset", namespace+"/"+name)
		return nil
	}

	ss, err := h.listers.SS.StatefulSets(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("statefulset", namespace+"/"+name)
			return nil
		}
		return fmt.Errorf(
			"failed to get statefulset %s/%s from cache: %w",
			namespace,
			name,
			err,
		)
	}

	return h.ProcessStatefulSetObject(ss, false)
}

func (h *handler) ProcessStatefulSetObject(
	ss *appsv1.StatefulSet,
	deleted bool,
) error {
	if ss == nil {
		return nil
	}

	if deleted {
		h.clearFirstUnavailableSts(ss.Namespace + "/" + ss.Name)
		h.correlator.ResolveByResource("statefulset", ss.Namespace+"/"+ss.Name)
		return nil
	}

	key := ss.Namespace + "/" + ss.Name
	conditionSignals := DetectStatefulSetConditions(ss)
	for _, sig := range conditionSignals {
		h.signalEvent(sig)
	}
	if len(conditionSignals) == 0 {
		h.correlator.MarkResolved(correlation.BuildKey(
			ss.Namespace, key, constant.ReasonStatefulSetCondition, "",
		))
	}

	if statefulSetUnavailable(ss) {
		first := h.markFirstUnavailableSts(key)

		settled := ss.Status.ObservedGeneration >= ss.Generation &&
			ss.Status.CurrentReplicas == ss.Status.Replicas
		if !settled {
			rolloutGrace := 15 * time.Minute
			if h.now().Sub(first) < rolloutGrace {
				return nil
			}
		}

		sustained := time.Duration(
			h.config.StatefulSetMonitor.SustainedMinutes,
		) * time.Minute
		if sustained > 0 && h.now().Sub(first) < sustained {
			return nil
		}

		h.signalEvent(&event.Signal{
			Resource:  "statefulset",
			Namespace: ss.Namespace,
			Reason:    constant.ReasonStsUnavailable,
			Owner:     key,
			Labels:    ss.Labels,
			Hint:      stsAvailabilityHint(ss),
		})
		return nil
	}
	h.clearFirstUnavailableSts(key)
	if len(conditionSignals) == 0 {
		h.correlator.ResolveByResource("statefulset", key)
	}
	return nil
}

func statefulSetReplicas(ss *appsv1.StatefulSet) int32 {
	if ss == nil {
		return 0
	}
	if ss.Spec.Replicas == nil {
		return ss.Status.Replicas
	}
	return *ss.Spec.Replicas
}

func statefulSetUnavailable(ss *appsv1.StatefulSet) bool {
	if ss == nil {
		return false
	}
	if ss.Spec.Replicas == nil {
		return ss.Status.Replicas > 0 && ss.Status.ReadyReplicas < ss.Status.Replicas
	}
	return *ss.Spec.Replicas > 0 && ss.Status.ReadyReplicas < *ss.Spec.Replicas
}

func (h *handler) markFirstUnavailableSts(key string) time.Time {
	return h.fs.unavailableSts.mark(key, h.now())
}

func (h *handler) clearFirstUnavailableSts(key string) {
	h.fs.unavailableSts.clear(key)
}
