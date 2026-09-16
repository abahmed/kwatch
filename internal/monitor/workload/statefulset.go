package workload

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// DetectStatefulSetIssue returns a finding for unavailable StatefulSet pods.
func DetectStatefulSetIssue(ss *appsv1.StatefulSet) *model.Observation {
	if ss == nil || !StatefulSetUnavailable(ss) {
		return nil
	}
	return observe.Object(
		"statefulset", ss, constant.ReasonStsUnavailable,
	).WithHint(StatefulSetAvailabilityHint(ss))
}

// DetectStatefulSetConditions returns findings for non-true conditions.
func DetectStatefulSetConditions(
	ss *appsv1.StatefulSet,
) []*model.Observation {
	if ss == nil {
		return nil
	}
	var observations []*model.Observation
	for _, condition := range ss.Status.Conditions {
		if condition.Status == corev1.ConditionTrue {
			continue
		}
		hint := string(condition.Type) + ": " + condition.Reason
		if condition.Message != "" {
			hint += " — " + condition.Message
		}
		observations = append(observations, observe.Object(
			"statefulset", ss, constant.ReasonStatefulSetCondition,
		).WithHint(hint))
	}
	return observations
}

// StatefulSetAvailabilityHint summarizes the StatefulSet replica shortfall.
func StatefulSetAvailabilityHint(ss *appsv1.StatefulSet) string {
	if ss == nil {
		return ""
	}
	desired := StatefulSetReplicas(ss)
	return fmt.Sprintf(
		"%d/%d pods not ready (ready: %d) — check PVC, pod status, or rollout "+
			"progress",
		desired-ss.Status.ReadyReplicas,
		desired,
		ss.Status.ReadyReplicas,
	)
}

// StatefulSetReplicas returns the configured or observed replica count.
func StatefulSetReplicas(ss *appsv1.StatefulSet) int32 {
	if ss == nil {
		return 0
	}
	if ss.Spec.Replicas != nil {
		return *ss.Spec.Replicas
	}
	return ss.Status.Replicas
}

// StatefulSetProcessor is the controller-facing contract for StatefulSets.
type StatefulSetProcessor interface {
	ProcessStatefulSet(string, bool) error
}

// StatefulSetConfig wires informer and clock dependencies.
type StatefulSetConfig interface {
	configureLister(appsv1lister.StatefulSetLister)
}

// StatefulSetRuntime owns StatefulSet lookup and availability lifecycle policy.
type StatefulSetRuntime struct {
	support runtimeSupport
	lister  appsv1lister.StatefulSetLister
	first   firstSeen
}

// NewStatefulSetRuntime constructs the direct StatefulSet family adapter.
// configureLister supplies the informer-backed StatefulSet cache.
func (r *StatefulSetRuntime) configureLister(
	lister appsv1lister.StatefulSetLister,
) {
	r.support.configureSource(func() { r.lister = lister })
}

// ProcessStatefulSet reconciles one StatefulSet queue key.
func (r *StatefulSetRuntime) ProcessStatefulSet(
	key string,
	deleted bool,
) error {
	return processKey(
		&r.support, key, "statefulset", deleted,
		nil,
		func() (appsv1lister.StatefulSetLister, bool) {
			lister := r.support.sourceSnapshot(
				func() appsv1lister.StatefulSetLister { return r.lister },
			)
			return lister, lister != nil
		},
		func(
			lister appsv1lister.StatefulSetLister,
			namespace, name string,
		) (*appsv1.StatefulSet, error) {
			return lister.StatefulSets(namespace).Get(name)
		},
		func(subject model.ObjectRef) {
			r.clearFirst(subject.Namespace + "/" + subject.Name)
			r.support.reconcileGone(subject)
		},
		func(subject model.ObjectRef, ss *appsv1.StatefulSet) error {
			return r.processStatefulSetObject(subject, ss)
		},
	)
}

func (r *StatefulSetRuntime) processStatefulSetObject(
	subject model.ObjectRef,
	ss *appsv1.StatefulSet,
) error {
	if ss == nil {
		return nil
	}
	key := ss.Namespace + "/" + ss.Name
	if r.support.maintenance(ss.Annotations) {
		r.clearFirst(key)
		r.support.reconcileGone(subject)
		return nil
	}
	current := DetectStatefulSetConditions(ss)
	if StatefulSetUnavailable(ss) {
		first := r.first.mark(key, r.support.now())
		if r.unavailableSustained(ss, first) {
			current = append(current, DetectStatefulSetIssue(ss))
		} else {
			for _, obs := range current {
				r.support.observe(obs)
			}
			return nil
		}
	} else {
		r.clearFirst(key)
	}
	r.support.reconcile(subject, current)
	return nil
}

func (r *StatefulSetRuntime) unavailableSustained(
	ss *appsv1.StatefulSet,
	first time.Time,
) bool {
	settled := ss.Status.ObservedGeneration >= ss.Generation &&
		ss.Status.CurrentReplicas == ss.Status.Replicas
	if !settled && r.support.now().Sub(first) < 15*time.Minute {
		return false
	}
	sustained := adaptiveSustained(
		r.support.runtime.Monitors().StatefulSet().SustainedMinutes,
		r.support.runtime.Monitors().AdaptiveThresholds(),
		StatefulSetReplicas(ss),
		StatefulSetReplicas(ss)-ss.Status.ReadyReplicas,
	)
	return sustained <= 0 || r.support.now().Sub(first) >= sustained
}

func (r *StatefulSetRuntime) clearFirst(key string) {
	r.first.clear(key)
}

// StatefulSetUnavailable reports whether a StatefulSet has missing replicas.
func StatefulSetUnavailable(ss *appsv1.StatefulSet) bool {
	if ss == nil {
		return false
	}
	if ss.Spec.Replicas == nil {
		return ss.Status.Replicas > 0 &&
			ss.Status.ReadyReplicas < ss.Status.Replicas
	}
	return *ss.Spec.Replicas > 0 &&
		ss.Status.ReadyReplicas < *ss.Spec.Replicas
}
