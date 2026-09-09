package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// DetectStatefulSetIssue returns a Signal if the StatefulSet has unavailable
// pods that would trigger an alert. Used for baseline seeding at startup.
func DetectStatefulSetIssue(ss *appsv1.StatefulSet) *model.Observation {
	if ss == nil {
		return nil
	}
	if statefulSetUnavailable(ss) {
		return observe.Object(
			"statefulset", ss, constant.ReasonStsUnavailable,
		).WithHint(stsAvailabilityHint(ss))
	}
	return nil
}

func DetectStatefulSetConditions(
	ss *appsv1.StatefulSet,
) []*model.Observation {
	if ss == nil {
		return nil
	}
	var out []*model.Observation
	for _, condition := range ss.Status.Conditions {
		if condition.Status == corev1.ConditionTrue {
			continue
		}
		hint := string(condition.Type) + ": " + condition.Reason
		if condition.Message != "" {
			hint += " — " + condition.Message
		}
		out = append(out, observe.Object(
			"statefulset", ss, constant.ReasonStatefulSetCondition,
		).WithHint(hint))
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
		h.reconcileGone(model.NewObjectRef("statefulset", namespace, name))
		return nil
	}

	ss, err := h.listers.SS.StatefulSets(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.reconcileGone(
				model.NewObjectRef("statefulset", namespace, name),
			)
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

	subject := model.NewObjectRef("statefulset", ss.Namespace, ss.Name)
	key := ss.Namespace + "/" + ss.Name
	if deleted || h.inMaintenance(ss.Annotations) {
		h.clearFirstUnavailableSts(key)
		h.reconcileGone(subject)
		return nil
	}

	current := DetectStatefulSetConditions(ss)

	if statefulSetUnavailable(ss) {
		first := h.markFirstUnavailableSts(key)
		if h.stsUnavailableSustained(ss, first) {
			current = append(current, observe.Object(
				"statefulset", ss, constant.ReasonStsUnavailable,
			).WithHint(stsAvailabilityHint(ss)))
		} else {
			// Still inside the rollout grace or the sustain window: report
			// nothing, and leave what is already open alone. Reconciling an
			// empty set here would resolve a condition that has not changed.
			for _, obs := range current {
				h.observe(obs)
			}
			return nil
		}
	} else {
		h.clearFirstUnavailableSts(key)
	}

	h.reconcile(subject, current)
	return nil
}

// stsUnavailableSustained reports whether an unavailable StatefulSet has been
// unavailable long enough to be worth an alert: past the rollout grace when it
// is mid-rollout, and past the configured sustain window either way.
func (h *handler) stsUnavailableSustained(
	ss *appsv1.StatefulSet, first time.Time,
) bool {
	settled := ss.Status.ObservedGeneration >= ss.Generation &&
		ss.Status.CurrentReplicas == ss.Status.Replicas
	if !settled {
		const rolloutGrace = 15 * time.Minute
		if h.now().Sub(first) < rolloutGrace {
			return false
		}
	}
	sustained := adaptiveSustained(
		h.config.StatefulSetMonitor.SustainedMinutes,
		h.config.AdaptiveThresholds,
		statefulSetReplicas(ss),
		statefulSetReplicas(ss)-ss.Status.ReadyReplicas,
	)
	return sustained <= 0 || h.now().Sub(first) >= sustained
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
