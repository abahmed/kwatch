package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
)

// DetectDaemonSetIssue returns a Signal if the DaemonSet has unavailable
// pods that would trigger an alert. Used for baseline seeding at startup.
func DetectDaemonSetIssue(ds *appsv1.DaemonSet) *event.Signal {
	if ds.Status.DesiredNumberScheduled > 0 && ds.Status.NumberUnavailable > 0 {
		settled := ds.Status.ObservedGeneration >= ds.Generation &&
			ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled
		// For baseline, only seed settled-unavailable or stuck-past-grace.
		// (startup baseline doesn't have timing info, so we seed unsettled
		// too — the engine's BaselineTTL will age them out before the grace
		// window matters.)
		if !settled {
			// Still seed it — the baseline TTL will handle expiry before
			// the grace window matters for a genuinely new rollout.
		}
		return &event.Signal{
			Resource:  "daemonset",
			Reason:    "DaemonSetUnavailable",
			Namespace: ds.Namespace,
			Owner:     ds.Namespace + "/" + ds.Name,
			Labels:    ds.Labels,
			Hint:      availabilityHint(ds),
		}
	}
	return nil
}

func availabilityHint(ds *appsv1.DaemonSet) string {
	unavailable := ds.Status.NumberUnavailable
	desired := ds.Status.DesiredNumberScheduled
	available := ds.Status.NumberAvailable
	return fmt.Sprintf("%d/%d pods unavailable (available: %d) — check node capacity, taints, or image",
		unavailable, desired, available)
}

func (h *handler) ProcessDaemonSet(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid daemonset key %q: %w", key, err)
	}

	if deleted {
		h.correlator.ResolveByResource("daemonset", namespace+"/"+name)
		return nil
	}

	ds, err := h.dsLister.DaemonSets(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("daemonset", namespace+"/"+name)
			return nil
		}
		return fmt.Errorf("failed to get daemonset %s/%s from cache: %w", namespace, name, err)
	}

	return h.ProcessDaemonSetObject(ds, false)
}

func (h *handler) ProcessDaemonSetObject(ds *appsv1.DaemonSet, deleted bool) error {
	if ds == nil {
		return nil
	}

	if deleted {
		h.clearFirstUnavailableDS(ds.Namespace + "/" + ds.Name)
		h.correlator.ResolveByResource("daemonset", ds.Namespace+"/"+ds.Name)
		return nil
	}

	key := ds.Namespace + "/" + ds.Name

	if ds.Status.DesiredNumberScheduled > 0 && ds.Status.NumberUnavailable > 0 {
		// Node-driven inhibition: if there are at least as many active node
		// incidents as unavailable DS pods, the root cause is the node, not
		// the DaemonSet — suppress to avoid duplicative alerts.
		if h.correlator.CountActiveNodeIncidents() >= int(ds.Status.NumberUnavailable) {
			h.clearFirstUnavailableDS(key)
			h.correlator.ResolveByResource("daemonset", key)
			return nil
		}

		first := h.markFirstUnavailableDS(key)

		// Only alert on sustained unavailability — rolling updates and brief
		// node blips have transient unavailability that should not page.
		// A DaemonSet stuck mid-rollout (unsettled) is given a grace window
		// before alerting; after the grace expires we assume it's stuck.
		settled := ds.Status.ObservedGeneration >= ds.Generation &&
			ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled
		if !settled {
			rolloutGrace := 15 * time.Minute
			if h.now().Sub(first) < rolloutGrace {
				return nil // genuinely mid-rollout — give it time
			}
			// still unsettled past the grace → stuck rollout; fall through
		}

		sustained := time.Duration(h.config.DaemonSetMonitor.SustainedMinutes) * time.Minute
		if sustained > 0 && h.now().Sub(first) < sustained {
			return nil
		}

		h.signalEvent(&event.Signal{
			Resource:  "daemonset",
			Namespace: ds.Namespace,
			Reason:    "DaemonSetUnavailable",
			Owner:     key,
			Labels:    ds.Labels,
			Hint:      availabilityHint(ds),
		})
		return nil
	}

	h.clearFirstUnavailableDS(key)
	h.correlator.ResolveByResource("daemonset", key)
	return nil
}

func (h *handler) markFirstUnavailableDS(key string) time.Time {
	h.dsMu.Lock()
	defer h.dsMu.Unlock()
	if t, ok := h.firstUnavailableDS[key]; ok {
		return t
	}
	h.firstUnavailableDS[key] = h.now()
	return h.firstUnavailableDS[key]
}

func (h *handler) clearFirstUnavailableDS(key string) {
	h.dsMu.Lock()
	defer h.dsMu.Unlock()
	delete(h.firstUnavailableDS, key)
}
