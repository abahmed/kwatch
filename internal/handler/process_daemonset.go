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

// DetectDaemonSetIssue returns a Signal if the DaemonSet has unavailable
// pods that would trigger an alert. Used for baseline seeding at startup.
func DetectDaemonSetIssue(ds *appsv1.DaemonSet) *model.Observation {
	if ds == nil {
		return nil
	}
	if ds.Status.DesiredNumberScheduled > 0 && ds.Status.NumberUnavailable > 0 {
		return observe.Object(
			"daemonset", ds, constant.ReasonDaemonSetUnavailable,
		).WithHint(availabilityHint(ds))
	}
	return nil
}

func DetectDaemonSetConditions(ds *appsv1.DaemonSet) []*model.Observation {
	if ds == nil {
		return nil
	}
	var out []*model.Observation
	for _, condition := range ds.Status.Conditions {
		if condition.Status == corev1.ConditionTrue {
			continue
		}
		hint := string(condition.Type) + ": " + condition.Reason
		if condition.Message != "" {
			hint += " — " + condition.Message
		}
		out = append(out, observe.Object(
			"daemonset", ds, constant.ReasonDaemonSetCondition,
		).WithHint(hint))
	}
	return out
}

func availabilityHint(ds *appsv1.DaemonSet) string {
	unavailable := ds.Status.NumberUnavailable
	desired := ds.Status.DesiredNumberScheduled
	available := ds.Status.NumberAvailable
	return fmt.Sprintf(
		"%d/%d pods unavailable (available: %d) — check node capacity, "+
			"taints, or image",
		unavailable,
		desired,
		available,
	)
}

func (h *handler) ProcessDaemonSet(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid daemonset key %q: %w", key, err)
	}

	if deleted {
		h.reconcileGone(model.NewObjectRef("daemonset", namespace, name))
		return nil
	}

	ds, err := h.listers.DS.DaemonSets(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.reconcileGone(model.NewObjectRef("daemonset", namespace, name))
			return nil
		}
		return fmt.Errorf(
			"failed to get daemonset %s/%s from cache: %w",
			namespace,
			name,
			err,
		)
	}

	return h.ProcessDaemonSetObject(ds, false)
}

func (h *handler) ProcessDaemonSetObject(
	ds *appsv1.DaemonSet,
	deleted bool,
) error {
	if ds == nil {
		return nil
	}

	subject := model.NewObjectRef("daemonset", ds.Namespace, ds.Name)
	key := ds.Namespace + "/" + ds.Name
	if deleted || h.inMaintenance(ds.Annotations) {
		h.clearFirstUnavailableDS(key)
		h.reconcileGone(subject)
		return nil
	}

	current := DetectDaemonSetConditions(ds)

	if ds.Status.DesiredNumberScheduled > 0 && ds.Status.NumberUnavailable > 0 {
		// Node-driven inhibition: if there are at least as many active node
		// incidents as unavailable DS pods, the root cause is the node, not
		// the DaemonSet — suppress to avoid duplicative alerts.
		if h.correlator.CountActiveNodeIncidents() >= int(
			ds.Status.NumberUnavailable,
		) {
			h.clearFirstUnavailableDS(key)
			h.reconcile(subject, current)
			return nil
		}

		first := h.markFirstUnavailableDS(key)
		if h.dsUnavailableSustained(ds, first) {
			current = append(current, observe.Object(
				"daemonset", ds, constant.ReasonDaemonSetUnavailable,
			).WithHint(availabilityHint(ds)))
		} else {
			// Mid-rollout or still inside the sustain window: report the
			// conditions that stand, and leave everything else as it is.
			for _, obs := range current {
				h.observe(obs)
			}
			return nil
		}
	} else {
		h.clearFirstUnavailableDS(key)
	}

	h.reconcile(subject, current)
	return nil
}

// dsUnavailableSustained reports whether an unavailable DaemonSet has been
// unavailable long enough to alert on. Rolling updates and brief node blips
// are transient and must not page: a DaemonSet still mid-rollout gets a grace
// window, and after it expires the rollout is treated as stuck.
func (h *handler) dsUnavailableSustained(
	ds *appsv1.DaemonSet, first time.Time,
) bool {
	settled := ds.Status.ObservedGeneration >= ds.Generation &&
		ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled
	if !settled {
		const rolloutGrace = 15 * time.Minute
		if h.now().Sub(first) < rolloutGrace {
			return false
		}
	}
	unavailable := ds.Status.DesiredNumberScheduled - ds.Status.NumberReady
	sustained := adaptiveSustained(
		h.config.DaemonSetMonitor.SustainedMinutes,
		h.config.AdaptiveThresholds,
		ds.Status.DesiredNumberScheduled,
		unavailable,
	)
	return sustained <= 0 || h.now().Sub(first) >= sustained
}

func (h *handler) markFirstUnavailableDS(key string) time.Time {
	return h.fs.unavailableDS.mark(key, h.now())
}

func (h *handler) clearFirstUnavailableDS(key string) {
	h.fs.unavailableDS.clear(key)
}
