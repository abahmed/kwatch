package correlation

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

// crossedTier returns the highest index of a tier whose threshold was
// crossed when moving from prev to new restarts, or -1.
func crossedTier(prev, new int, tiers []int) int {
	hit := -1
	for i, t := range tiers {
		if prev < t && new >= t {
			hit = i
		}
	}
	return hit
}

// severityForTier returns the severity for the given escalation tier index,
// preferring the higher of the tier-based severity and the current severity.
func severityForTier(tierIdx int, current model.Severity) model.Severity {
	sev := model.SeverityCritical
	if tierIdx == 0 {
		sev = model.SeverityHigh
	}
	if current.Rank() > sev.Rank() {
		return current
	}
	return sev
}

// escalateRestartCount applies escalation tiers when the incident's restart
// count crosses a configured threshold, bumping severity and re-enriching.
// Caller must hold e.mu.
func (e *Engine) escalateRestartCount(inc *model.Incident, ev event.Event, cs *model.ContainerState, now time.Time) bool {
	prev := inc.RestartCount
	cur := int(cs.RestartCount)
	t := crossedTier(prev, cur, e.config.EscalationTiers)
	if t < 0 {
		return false
	}
	ev.Severity = severityForTier(t, inc.Severity)
	e.config.Enricher.Enrich(&ev, inc)
	inc.Hint = fmt.Sprintf("restart count crossed %d", e.config.EscalationTiers[t])
	inc.Count++
	inc.LastSeen = now
	inc.State = model.StateActive
	inc.LastUpdate = now
	inc.RestartCount = cur
	if ev.PodName != "" {
		inc.Resources[ev.PodName] = true
		if len(inc.Resources) > inc.PeakResources {
			inc.PeakResources = len(inc.Resources)
		}
	}
	if ev.ContainerName != "" && ev.ContainerName != "." {
		inc.Containers[ev.ContainerName] = true
	}
	inc.LastContainerState = cs
	e.indexLastContainerState(ev.Namespace, ev.PodName, ev.ContainerName, cs)
	return true
}

// foldCrashLoopIncident migrates an existing incident to the high-frequency
// crash-loop key when the container's restart count crosses the threshold,
// carrying over baseline entries and group membership. Caller must hold
// e.mu.
func (e *Engine) foldCrashLoopIncident(ev event.Event, owner string, key model.IncidentKey, res string) (*model.Incident, bool) {
	var orphan *model.Incident
	for k, oldInc := range e.state {
		if k == key ||
			oldInc.State == model.StateResolved ||
			oldInc.State == model.StatePendingResolve ||
			oldInc.Resource != res {
			continue
		}
		pk := ParseKey(k)
		if pk.Namespace != ev.Namespace || pk.Owner != owner {
			continue
		}
		orphan = oldInc
		e.removeIncidentFromNamespaceIndex(oldInc)
		delete(e.state, k)
		if pods, ok := e.baseline[string(k)]; ok {
			if e.baseline[string(key)] == nil {
				e.baseline[string(key)] = make(map[string]int64, len(pods))
			}
			for pod, ts := range pods {
				e.baseline[string(key)][pod] = ts
			}
			delete(e.baseline, string(k))
		}
		// Follow any smart-group membership onto the new key so the
		// group isn't left waiting on a member that no longer exists.
		e.rekeyGroupReferences(k, key)
		break
	}
	if orphan == nil {
		return nil, false
	}
	orphan.Key = key
	orphan.Reason = normalizeReason(ev.Reason)
	orphan.State = model.StateActive
	orphan.ResolveAt = time.Time{}
	e.state[key] = orphan
	e.indexIncidentByNamespace(orphan)
	return orphan, true
}
