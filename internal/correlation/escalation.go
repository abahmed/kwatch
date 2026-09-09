package correlation

import (
	"fmt"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/enricher"
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
func (e *Engine) escalateRestartCount(
	inc *model.Incident,
	ev event.Event,
	cs *model.ContainerState,
	now time.Time,
) bool {
	prev := inc.RestartCount
	cur := int(cs.RestartCount)
	t := crossedTier(prev, cur, e.config.EscalationTiers)
	if t < 0 {
		return false
	}
	ev.Severity = severityForTier(t, inc.Severity)
	e.config.Enricher.Enrich(&ev, inc)
	// The escalation is added to the diagnosis, not put in its place. It used
	// to overwrite the hint, so the moment an incident got worse the alert
	// stopped saying what was wrong with it and said only that the restart
	// count had gone up.
	inc.Hint = enricher.CombineHints(
		inc.Hint,
		fmt.Sprintf("restart count crossed %d", e.config.EscalationTiers[t]),
	)
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

// crashLoopFoldReasons are the reasons a high-frequency crash loop is the
// same incident as. They are exactly the reasons IncidentKey rewrites to
// CrashLoopHighFreq, which is what makes the fold a rename rather than a
// merge of two different problems.
var crashLoopFoldReasons = map[string]bool{
	constant.ReasonError:             true,
	constant.ReasonOOMKilled:         true,
	constant.ReasonCrashLoopBackOff:  true,
	constant.ReasonCrashLoopHighFreq: true,
}

// foldCrashLoopIncident migrates an existing incident to the high-frequency
// crash-loop key when the container's restart count crosses the threshold,
// carrying over baseline entries and group membership. Caller must hold
// e.mu.
//
// Two things decide which incident is the same one under a new name. The
// owner must match the owner slot the new key was built from -- comparing
// against the raw owner instead meant an ownerless pod, whose key owner is a
// UID or lineage hash, never matched its own incident and got a duplicate
// CREATE alongside an orphan. And the reason must be one the fold actually
// renames: without that check an ImagePullBackOff incident for the same
// workload was picked up and silently relabelled as a crash loop.
func (e *Engine) foldCrashLoopIncident(
	ev event.Event,
	key model.IncidentKey,
	res string,
) (*model.Incident, bool) {
	k, orphan := e.crashLoopFoldCandidate(ev, ParseKey(key).Owner, key, res)
	if orphan == nil {
		return nil, false
	}
	e.unindexIncident(orphan)
	delete(e.state, k)
	if pods, ok := e.baseline[string(k)]; ok {
		folded := e.baselineBucket(string(key))
		for pod, ts := range pods {
			folded[pod] = ts
		}
		e.dropBaselineKey(string(k))
	}
	// Follow any smart-group membership onto the new key so the group isn't
	// left waiting on a member that no longer exists.
	e.rekeyGroupReferences(k, key)
	orphan.Key = key
	orphan.Reason = normalizeReason(ev.Reason)
	orphan.State = model.StateActive
	orphan.ResolveAt = time.Time{}
	e.state[key] = orphan
	e.indexIncident(orphan)
	return orphan, true
}

// crashLoopFoldCandidate picks the incident the folded key continues.
//
// Keys are walked in sorted order and a candidate that already names this
// container wins outright, so the choice is the same on every replica of
// kwatch and on every replay of the same events -- map order used to decide
// it, which meant two containers of one workload crash-looping could fold
// into each other's incident depending on the run. Caller must hold e.mu.
func (e *Engine) crashLoopFoldCandidate(
	ev event.Event,
	keyOwner string,
	key model.IncidentKey,
	res string,
) (model.IncidentKey, *model.Incident) {
	keys := make([]string, 0, len(e.state))
	for k := range e.state {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)

	var bestKey model.IncidentKey
	var best *model.Incident
	for _, raw := range keys {
		k := model.IncidentKey(raw)
		inc := e.state[k]
		if k == key || inc.Resource != res ||
			inc.State == model.StateResolved ||
			inc.State == model.StatePendingResolve {
			continue
		}
		if !crashLoopFoldReasons[normalizeReason(inc.Reason)] {
			continue
		}
		pk := ParseKey(k)
		if pk.Namespace != ev.Namespace || pk.Owner != keyOwner {
			continue
		}
		if ev.ContainerName != "" && inc.Containers[ev.ContainerName] {
			return k, inc
		}
		if best == nil {
			bestKey, best = k, inc
		}
	}
	return bestKey, best
}
