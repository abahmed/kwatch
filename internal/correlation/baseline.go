package correlation

import (
	"sort"
	"strings"
	"time"
)

func (e *Engine) SetSeen(b map[string]map[string]int64) {
	e.mu.Lock()
	e.dirty = true
	now := e.now()
	ttl := e.config.BaselineTTL
	if e.seen == nil {
		e.seen = make(map[string]map[string]int64)
	}
	for key, pods := range b {
		for pod, ts := range pods {
			if now.Sub(time.Unix(ts, 0)) < ttl {
				if e.seen[key] == nil {
					e.seen[key] = map[string]int64{}
				}
				e.seen[key][pod] = ts
			}
		}
	}
	e.evictToLimit()
	snap := cloneBaseline(e.seen)
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
	for _, pods := range e.seen {
		total += len(pods)
	}
	if total <= limit {
		return
	}

	var all []seenEntry
	for key, pods := range e.seen {
		for pod, ts := range pods {
			all = append(all, seenEntry{key, pod, ts})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].ts < all[j].ts
	})

	toRemove := total - limit
	for _, entry := range all[:toRemove] {
		if pods, ok := e.seen[entry.key]; ok {
			delete(pods, entry.pod)
			if len(pods) == 0 {
				delete(e.seen, entry.key)
			}
		}
	}
}

// Caller must hold e.mu.
func (e *Engine) isBaselined(key, podName string) bool {
	if pods, ok := e.seen[key]; ok {
		if ts, ok := pods[podName]; ok {
			if e.now().Sub(time.Unix(ts, 0)) < e.config.BaselineTTL {
				return true
			}
			delete(pods, podName)
			if len(pods) == 0 {
				delete(e.seen, key)
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

// ClearSeenForPod removes all baseline entries and cooldowns for the given pod.
func (e *Engine) ClearSeenForPod(namespace, podName string) {
	e.mu.Lock()
	e.dirty = true
	changed := false
	for key, pods := range e.seen {
		if !strings.HasPrefix(key, namespace+":") {
			continue
		}
		if _, ok := pods[podName]; ok {
			delete(pods, podName)
			changed = true
			if len(pods) == 0 {
				delete(e.seen, key)
			}
		}
	}
	// Clear any cleanup cooldown entries for this namespace so a recovered
	// pod that re-crashes isn't suppressed by a stale cooldown.
	nsPrefix := namespace + ":"
	for key := range e.cleanupCooldown {
		if strings.HasPrefix(key, nsPrefix) {
			delete(e.cleanupCooldown, key)
		}
	}
	var snap map[string]map[string]int64
	if changed {
		snap = cloneBaseline(e.seen)
	}
	e.mu.Unlock()
	if changed && e.config.OnBaselineChange != nil {
		e.config.OnBaselineChange(snap)
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
