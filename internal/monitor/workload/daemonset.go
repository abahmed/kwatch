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

// DetectDaemonSetIssue returns a finding for unavailable DaemonSet pods.
func DetectDaemonSetIssue(ds *appsv1.DaemonSet) *model.Observation {
	if ds == nil {
		return nil
	}
	if ds.Status.DesiredNumberScheduled > 0 &&
		ds.Status.NumberUnavailable > 0 {
		return observe.Object(
			"daemonset", ds, constant.ReasonDaemonSetUnavailable,
		).WithHint(DaemonSetAvailabilityHint(ds)).WithFacts(model.Facts{
			DesiredReplicas: ds.Status.DesiredNumberScheduled,
			ReadyReplicas:   ds.Status.NumberReady,
		})
	}
	return nil
}

// DetectDaemonSetConditions returns findings for non-true conditions.
func DetectDaemonSetConditions(ds *appsv1.DaemonSet) []*model.Observation {
	if ds == nil {
		return nil
	}
	var observations []*model.Observation
	for _, condition := range ds.Status.Conditions {
		if condition.Status == corev1.ConditionTrue {
			continue
		}
		hint := string(condition.Type) + ": " + condition.Reason
		if condition.Message != "" {
			hint += " — " + condition.Message
		}
		observations = append(observations, observe.Object(
			"daemonset", ds, constant.ReasonDaemonSetCondition,
		).WithHint(hint))
	}
	return observations
}

// DaemonSetAvailabilityHint summarizes unavailable DaemonSet pods.
func DaemonSetAvailabilityHint(ds *appsv1.DaemonSet) string {
	if ds == nil {
		return ""
	}
	return fmt.Sprintf(
		"%d/%d pods unavailable (available: %d) — check node capacity, "+
			"taints, or image",
		ds.Status.NumberUnavailable,
		ds.Status.DesiredNumberScheduled,
		ds.Status.NumberAvailable,
	)
}

// DaemonSetProcessor is the controller-facing contract for DaemonSets.
type DaemonSetProcessor interface {
	ProcessDaemonSet(string, bool) error
}

// DaemonSetConfig wires informer, clock, and sustain dependencies.
type DaemonSetConfig interface {
	configureLister(appsv1lister.DaemonSetLister)
}

// DaemonSetRuntime owns DaemonSet lookup and availability lifecycle policy.
type DaemonSetRuntime struct {
	support runtimeSupport
	lister  appsv1lister.DaemonSetLister
	first   firstSeen
}

// NewDaemonSetRuntime constructs the direct DaemonSet family adapter.
// configureLister supplies the informer-backed DaemonSet cache.
func (r *DaemonSetRuntime) configureLister(
	lister appsv1lister.DaemonSetLister,
) {
	r.support.configureSource(func() { r.lister = lister })
}

// ProcessDaemonSet reconciles one DaemonSet queue key.
func (r *DaemonSetRuntime) ProcessDaemonSet(
	key string,
	deleted bool,
) error {
	return processKey(
		&r.support, key, "daemonset", deleted,
		nil,
		func() (appsv1lister.DaemonSetLister, bool) {
			lister := r.support.sourceSnapshot(
				func() appsv1lister.DaemonSetLister { return r.lister },
			)
			return lister, lister != nil
		},
		func(
			lister appsv1lister.DaemonSetLister,
			namespace, name string,
		) (*appsv1.DaemonSet, error) {
			return lister.DaemonSets(namespace).Get(name)
		},
		func(subject model.ObjectRef) {
			r.clearFirst(subject.Namespace + "/" + subject.Name)
			r.support.reconcileGone(subject)
		},
		func(subject model.ObjectRef, ds *appsv1.DaemonSet) error {
			return r.processDaemonSetObject(subject, ds)
		},
	)
}

func (r *DaemonSetRuntime) processDaemonSetObject(
	subject model.ObjectRef,
	ds *appsv1.DaemonSet,
) error {
	if ds == nil {
		return nil
	}
	key := ds.Namespace + "/" + ds.Name
	if r.support.maintenance(ds.Annotations) {
		r.clearFirst(key)
		r.support.reconcileGone(subject)
		return nil
	}
	current := DetectDaemonSetConditions(ds)
	if ds.Status.DesiredNumberScheduled > 0 &&
		ds.Status.NumberUnavailable > 0 {
		if r.support.activeNodeIncidentCount != nil &&
			r.support.activeNodeIncidentCount() >= int(
				ds.Status.NumberUnavailable,
			) {
			r.clearFirst(key)
			r.support.reconcile(subject, current)
			return nil
		}
		first := r.first.mark(key, r.support.now())
		if r.unavailableSustained(ds, first) {
			current = append(
				current,
				DetectDaemonSetIssue(ds),
			)
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

func (r *DaemonSetRuntime) unavailableSustained(
	ds *appsv1.DaemonSet,
	first time.Time,
) bool {
	settled := ds.Status.ObservedGeneration >= ds.Generation &&
		ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled
	if !settled && r.support.now().Sub(first) < 15*time.Minute {
		return false
	}
	unavailable := ds.Status.DesiredNumberScheduled - ds.Status.NumberReady
	sustained := adaptiveSustained(
		r.support.runtime.Monitors().DaemonSet().SustainedMinutes,
		r.support.runtime.Monitors().AdaptiveThresholds(),
		ds.Status.DesiredNumberScheduled,
		unavailable,
	)
	return sustained <= 0 || r.support.now().Sub(first) >= sustained
}

func (r *DaemonSetRuntime) clearFirst(key string) {
	r.first.clear(key)
}
