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
				if e.baseline[key] == nil {
					e.baseline[key] = map[string]int64{}
				}
				e.baseline[key][pod] = ts
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
			if len(pods) == 0 {
				delete(e.baseline, entry.key)
			}
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
			if len(pods) == 0 {
				delete(e.baseline, string(key))
			}
		}
		// Owner-level baseline (seeded with an empty pod name) covers every
		// pod of the owner, even when the live signal carries a pod name.
		if ts, ok := pods[""]; ok {
			if e.now().Sub(time.Unix(ts, 0)) < e.config.BaselineTTL {
				return true
			}
		}
	}
	return false
}

// ClearBaselineForPod removes all baseline entries and cooldowns for the given pod.
func (e *Engine) ClearBaselineForPod(namespace, podName string) {
	e.mu.Lock()
	e.dirty = true
	changed := false
	for key, pods := range e.baseline {
		if !strings.HasPrefix(key, namespace+":") {
			continue
		}
		if _, ok := pods[podName]; ok {
			delete(pods, podName)
			changed = true
			if len(pods) == 0 {
				delete(e.baseline, key)
			}
		}
	}
	// Clear any cleanup cooldown entries for this namespace so a recovered
	// pod that re-crashes isn't suppressed by a stale cooldown.
	nsPrefix := namespace + ":"
	for key := range e.cleanupCooldown {
		if strings.HasPrefix(string(key), nsPrefix) {
			delete(e.cleanupCooldown, key)
		}
	}
	var snap map[string]map[string]int64
	if changed {
		snap = cloneBaseline(e.baseline)
	}
	e.mu.Unlock()
	if changed && e.config.OnBaselineChange != nil {
		e.config.OnBaselineChange(snap)
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
		delete(pods, inc.Name)
	default:
		delete(pods, "")
		for pod := range inc.Resources {
			delete(pods, pod)
		}
	}
	if len(pods) == 0 {
		delete(e.baseline, string(key))
	}
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
