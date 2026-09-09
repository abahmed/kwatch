package correlation

import (
	"sort"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// maxPersistedContainerStates bounds the container-state half of the
// snapshot. The index is the largest of these maps on a busy cluster and the
// least valuable per entry, so it is the one that gets a cap: the freshest
// entries are the ones an alert is about to reference.
const maxPersistedContainerStates = 2000

// SnapshotEngineState returns the engine bookkeeping that is neither an
// incident nor a group. See model.PersistedEngineState for what each part
// costs when it is lost.
func (e *Engine) SnapshotEngineState() model.PersistedEngineState {
	e.mu.Lock()
	defer e.mu.Unlock()
	return model.PersistedEngineState{
		Cooldowns:  e.snapshotCooldowns(),
		PodUIDs:    e.snapshotPodUIDs(),
		Containers: e.snapshotContainerStates(),
		FanOut:     e.snapshotFanOutWindows(),
	}
}

// Caller must hold e.mu.
func (e *Engine) snapshotCooldowns() []model.PersistedCooldown {
	out := make([]model.PersistedCooldown, 0, len(e.cleanupCooldown))
	for key, expires := range e.cleanupCooldown {
		out = append(out, model.PersistedCooldown{Key: key, Expires: expires})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Caller must hold e.mu.
func (e *Engine) snapshotPodUIDs() []model.PersistedPodUIDs {
	out := make([]model.PersistedPodUIDs, 0, len(e.podResourceUIDs))
	for key, uids := range e.podResourceUIDs {
		if len(uids) == 0 {
			continue
		}
		copied := make(map[string]string, len(uids))
		for pod, uid := range uids {
			copied[pod] = uid
		}
		out = append(out, model.PersistedPodUIDs{Key: key, UIDs: copied})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Caller must hold e.mu.
func (e *Engine) snapshotContainerStates() []model.PersistedContainerState {
	out := make([]model.PersistedContainerState, 0, len(e.lastContainerIndex))
	for key, entry := range e.lastContainerIndex {
		if entry.state == nil {
			continue
		}
		cp := *entry.state
		out = append(out, model.PersistedContainerState{
			Key:       key,
			IndexedAt: entry.indexedAt,
			State:     &cp,
		})
	}
	// Freshest first, so the cap sheds the entries least likely to be read,
	// then by key so equal timestamps still serialize deterministically.
	sort.Slice(out, func(i, j int) bool {
		if !out[i].IndexedAt.Equal(out[j].IndexedAt) {
			return out[i].IndexedAt.After(out[j].IndexedAt)
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > maxPersistedContainerStates {
		out = out[:maxPersistedContainerStates]
	}
	return out
}

// Caller must hold e.mu.
func (e *Engine) snapshotFanOutWindows() []model.PersistedFanOutWindow {
	out := make([]model.PersistedFanOutWindow, 0, len(e.fanOutWindows))
	for scope, w := range e.fanOutWindows {
		if w == nil {
			continue
		}
		pw := model.PersistedFanOutWindow{
			Scope:     scope,
			FirstSeen: w.firstSeen,
		}
		for owner := range w.owners {
			pw.Owners = append(pw.Owners, owner)
		}
		sort.Strings(pw.Owners)
		if len(w.announced) > 0 {
			pw.Announced = make(map[string]model.IncidentKey, len(w.announced))
			for owner, key := range w.announced {
				pw.Announced[owner] = key
			}
		}
		out = append(out, pw)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Scope < out[j].Scope })
	return out
}

// RestoreEngineState reinstates the bookkeeping saved by
// SnapshotEngineState. Call it after RestoreIncidents: entries are only
// adopted while they are still meaningful, so nothing is resurrected for an
// incident that did not come back or a window that has already closed.
func (e *Engine) RestoreEngineState(saved model.PersistedEngineState) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	restored := 0
	for _, cd := range saved.Cooldowns {
		// An expired cooldown is not worth reviving, and reviving one would
		// silence a recurrence that should be announced.
		if cd.Key == "" || !now.Before(cd.Expires) {
			continue
		}
		e.cleanupCooldown[cd.Key] = cd.Expires
		restored++
	}
	for _, pu := range saved.PodUIDs {
		if pu.Key == "" || len(pu.UIDs) == 0 {
			continue
		}
		if _, live := e.state[pu.Key]; !live {
			continue
		}
		uids := make(map[string]string, len(pu.UIDs))
		for pod, uid := range pu.UIDs {
			uids[pod] = uid
		}
		e.podResourceUIDs[pu.Key] = uids
		restored++
	}
	restored += e.restoreContainerStates(saved.Containers, now)
	restored += e.restoreFanOutWindows(saved.FanOut, now)
	if restored > 0 {
		e.dirty = true
		klog.InfoS("restored engine state from ConfigMap", "entries", restored)
	}
}

// Caller must hold e.mu.
func (e *Engine) restoreContainerStates(
	saved []model.PersistedContainerState,
	now time.Time,
) int {
	restored := 0
	for _, cs := range saved {
		if cs.Key == "" || cs.State == nil {
			continue
		}
		// Honour the same TTL the live sweep applies, so a long outage does
		// not reload state the running engine would have dropped.
		if now.Sub(cs.IndexedAt) > containerStateTTL {
			continue
		}
		state := *cs.State
		e.lastContainerIndex[cs.Key] = containerStateEntry{
			state:     &state,
			indexedAt: cs.IndexedAt,
		}
		restored++
	}
	return restored
}

// Caller must hold e.mu.
func (e *Engine) restoreFanOutWindows(
	saved []model.PersistedFanOutWindow,
	now time.Time,
) int {
	restored := 0
	for _, pw := range saved {
		if pw.Scope == "" {
			continue
		}
		// A window that has already elapsed would be pruned on the next
		// flush anyway.
		if now.Sub(pw.FirstSeen) > e.config.SmartGroupingWindow {
			continue
		}
		w := &ownerWindow{
			firstSeen: pw.FirstSeen,
			owners:    make(map[string]bool, len(pw.Owners)),
			announced: make(map[string]model.IncidentKey, len(pw.Announced)),
		}
		for _, owner := range pw.Owners {
			w.owners[owner] = true
		}
		for owner, key := range pw.Announced {
			w.announced[owner] = key
		}
		e.fanOutWindows[pw.Scope] = w
		restored++
	}
	return restored
}
