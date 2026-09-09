package correlation

import (
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/model"
)

// SetBaseline loads a startup baseline captured by the controller. Keys follow
// the BuildKey format (see model.IncidentKey); the map is kept as raw strings
// because it crosses the ConfigMap persistence layer unchanged.
func (e *Engine) SetBaseline(b map[string]map[string]int64) {
	e.mu.Lock()
	e.dirty = true
	now := e.now()
	ttl := e.config.BaselineTTL
	if e.baseline == nil {
		e.baseline = make(map[string]map[string]int64)
	}
	for key, pods := range b {
		for pod, ts := range pods {
			if now.Sub(time.Unix(ts, 0)) < ttl {
				e.baselineBucket(key)[pod] = ts
			}
		}
	}
	e.evictToLimit()
	snap := cloneBaseline(e.baseline)
	e.mu.Unlock()
	if e.config.OnBaselineChange != nil {
		e.config.OnBaselineChange(snap)
	}
}

type seenEntry struct {
	key string
	pod string
	ts  int64
}

// Caller must hold e.mu.
func (e *Engine) evictToLimit() {
	limit := e.config.MaxBaseline
	total := 0
	for _, pods := range e.baseline {
		total += len(pods)
	}
	if total <= limit {
		return
	}

	var all []seenEntry
	for key, pods := range e.baseline {
		for pod, ts := range pods {
			all = append(all, seenEntry{key, pod, ts})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].ts < all[j].ts
	})

	toRemove := total - limit
	for _, entry := range all[:toRemove] {
		if pods, ok := e.baseline[entry.key]; ok {
			delete(pods, entry.pod)
			e.dropEmptyBaselineKey(entry.key)
		}
	}
}

// Caller must hold e.mu.
func (e *Engine) isBaselined(key model.IncidentKey, podName string) bool {
	if pods, ok := e.baseline[string(key)]; ok {
		if ts, ok := pods[podName]; ok {
			if e.now().Sub(time.Unix(ts, 0)) < e.config.BaselineTTL {
				return true
			}
			delete(pods, podName)
			e.dropEmptyBaselineKey(string(key))
		}
		// Owner-level baseline (seeded with an empty pod name) covers every
		// pod of the owner, even when the live signal carries a pod name. It
		// expires on its own short TTL -- see Config.OwnerBaselineTTL.
		if ts, ok := pods[""]; ok {
			if e.now().Sub(time.Unix(ts, 0)) < e.config.OwnerBaselineTTL {
				return true
			}
			delete(pods, "")
			e.dropEmptyBaselineKey(string(key))
		}
	}
	return false
}

// ClearBaselineForPod retires the suppression a now-healthy pod was
// justifying: its own startup-baseline entries, and the cleanup cooldowns of
// the incidents its own key can re-open.
//
// owner is the workload the pod's incidents are keyed by, and it is what
// scopes the cooldown half. Without it the sweep cleared every cooldown in
// the namespace, so one healthy pod disarmed every other incident's cooldown
// and the resolve-then-recreate flip-flop the cooldown exists to prevent was
// not prevented anywhere.
func (e *Engine) ClearBaselineForPod(
	namespace, podName string,
	owner model.ObjectRef,
) {
	e.mu.Lock()
	e.dirty = true
	changed := e.dropPodBaselineEntries(namespace, podName)
	e.clearPodCooldowns(namespace, podName, owner)
	var snap map[string]map[string]int64
	if changed {
		snap = cloneBaseline(e.baseline)
	}
	e.mu.Unlock()
	if changed && e.config.OnBaselineChange != nil {
		e.config.OnBaselineChange(snap)
	}
}

// dropPodBaselineEntries removes one pod's baseline entries across every key
// in its namespace, and reports whether anything was removed. Caller must
// hold e.mu.
func (e *Engine) dropPodBaselineEntries(namespace, podName string) bool {
	changed := false
	for key, pods := range e.baseline {
		if !strings.HasPrefix(key, namespace+":") {
			continue
		}
		if _, ok := pods[podName]; !ok {
			continue
		}
		delete(pods, podName)
		changed = true
		e.dropEmptyBaselineKey(key)
	}
	return changed
}

// clearPodCooldowns drops the cleanup cooldowns that this pod's recovery can
// answer for: they belong to its own owner, or to the pod itself when it has
// no owner, and to a reason a healthy pod actually falsifies. Cooldowns for
// other owners, and for the historical aggregates that a few healthy minutes
// do not disprove, are left running. Caller must hold e.mu.
func (e *Engine) clearPodCooldowns(
	namespace, podName string,
	owner model.ObjectRef,
) {
	for key := range e.cleanupCooldown {
		pk := ParseKey(key)
		if pk.Namespace != namespace {
			continue
		}
		if pk.Owner != podName &&
			(owner.Name == "" || pk.Owner != owner.Name) {
			continue
		}
		if !resolvableContainerReasons[pk.Reason] &&
			!resolvablePodReasons[pk.Reason] {
			continue
		}
		delete(e.cleanupCooldown, key)
		e.clearSkipAudit(key)
	}
}

// FilterBaseline removes entries outside the currently monitored namespace
// scope during startup restoration.
func (e *Engine) FilterBaseline(allowed func(string) bool) {
	if allowed == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for key := range e.baseline {
		ns := key
		if i := strings.IndexByte(ns, ':'); i >= 0 {
			ns = ns[:i]
		}
		if ns != "" && !allowed(ns) {
			e.dropBaselineKey(key)
			e.dirty = true
		}
	}
}

// removeBaselineForIncident drops the resolved incident's coverage from the
// baseline while preserving sibling entries. Deleting the whole key (as before)
// un-baselined sibling pods that share the same owner+reason key, so their
// pre-existing issues started re-alerting after an unrelated pod recovered.
//   - pod incidents cover specific pods → drop only those from seen[key]
//   - node incidents are keyed by node name → drop that entry
//   - workload incidents (deployment/statefulset/daemonset/job/...) carry an
//     owner-level baseline ("" seeded at startup) → drop it plus any pods the
//     incident accumulated
//
// Caller must hold e.mu.
func (e *Engine) removeBaselineForIncident(key model.IncidentKey, inc *model.Incident) {
	pods, ok := e.baseline[string(key)]
	if !ok {
		return
	}
	switch inc.Resource {
	case "pod":
		for pod := range inc.Resources {
			delete(pods, pod)
		}
	case "node":
		delete(pods, inc.Ref().Name)
	default:
		delete(pods, "")
		for pod := range inc.Resources {
			delete(pods, pod)
		}
	}
	e.dropEmptyBaselineKey(string(key))
}

func cloneBaseline(src map[string]map[string]int64) map[string]map[string]int64 {
	dst := make(map[string]map[string]int64, len(src))
	for k, pods := range src {
		m := make(map[string]int64, len(pods))
		for p, ts := range pods {
			m[p] = ts
		}
		dst[k] = m
	}
	return dst
}

// clearOwnerBaseline drops the owner-level baseline entries for one subject.
//
// A resolve only ever cleared the baseline through the incident it resolved.
// When the owner-level entry had suppressed the incident in the first place
// there was nothing to resolve, so the entry survived until its TTL and the
// next genuine failure of that workload stayed silent. Any observation of
// recovery must retire the suppression it justified. Caller must hold e.mu.
func (e *Engine) clearOwnerBaseline(namespace, owner string) bool {
	if owner == "" {
		return false
	}
	changed := false
	for _, key := range e.ownerBaselineKeys(namespace, owner) {
		pods := e.baseline[key]
		if _, ok := pods[""]; !ok {
			continue
		}
		delete(pods, "")
		changed = true
		e.dropEmptyBaselineKey(key)
	}
	return changed
}

// clearBaselineEntryForKey retires the owner-level baseline entry for one
// incident key. Caller must hold e.mu.
func (e *Engine) clearBaselineEntryForKey(key model.IncidentKey) bool {
	pods, ok := e.baseline[string(key)]
	if !ok {
		return false
	}
	if _, seeded := pods[""]; !seeded {
		return false
	}
	delete(pods, "")
	e.dropEmptyBaselineKey(string(key))
	return true
}
