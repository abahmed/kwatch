package correlation

import (
	"sort"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// SnapshotGroups returns the smart-group state to persist alongside the
// incidents.
//
// Without it a restart kept the incidents but forgot the groups speaking for
// them, so every member resolved individually and a group still failing
// announced itself again as new.
func (e *Engine) SnapshotGroups() []model.PersistedGroup {
	e.mu.Lock()
	defer e.mu.Unlock()

	keys := make([]string, 0, len(e.groupResolveTrackers))
	seen := make(map[string]bool, len(e.groupResolveTrackers))
	for gk := range e.groupResolveTrackers {
		keys = append(keys, gk)
		seen[gk] = true
	}
	for gk := range e.groupFlushStates {
		if !seen[gk] {
			keys = append(keys, gk)
		}
	}
	sort.Strings(keys)

	out := make([]model.PersistedGroup, 0, len(keys))
	for _, gk := range keys {
		pg := model.PersistedGroup{GroupKey: gk}
		if fs := e.groupFlushStates[gk]; fs != nil {
			pg.Notified = fs.notified
			pg.LastNotifiedAt = fs.lastNotifiedAt
			pg.FirstSeen = fs.firstSeen
		}
		if tracker := e.groupResolveTrackers[gk]; tracker != nil {
			pg.IncidentKey = tracker.groupIncKey
			pg.Reason = tracker.reason
			pg.Summary = tracker.summary
			pg.TotalCount = tracker.totalCount
			pg.LastSeen = tracker.lastSeen
			pg.Severity = tracker.severity
			if pg.FirstSeen.IsZero() {
				pg.FirstSeen = tracker.firstSeen
			}
			members := make([]model.IncidentKey, 0, len(tracker.members))
			for key := range tracker.members {
				members = append(members, key)
			}
			sort.Slice(members, func(i, j int) bool {
				return members[i] < members[j]
			})
			for _, key := range members {
				if tracker.members[key] {
					pg.Resolved = append(pg.Resolved, key)
					continue
				}
				pg.Members = append(pg.Members, key)
			}
		}
		out = append(out, pg)
	}
	return out
}

// RestoreGroups reinstates smart-group state after a restart. Call it after
// RestoreIncidents: a member whose incident did not come back is dropped, so a
// group never waits forever on something that no longer exists.
func (e *Engine) RestoreGroups(groups []model.PersistedGroup) {
	if len(groups) == 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dirty = true
	restored := 0
	for i := range groups {
		pg := &groups[i]
		if pg.GroupKey == "" {
			continue
		}
		if pg.Notified {
			e.groupFlushStates[pg.GroupKey] = &groupFlushState{
				notified:       true,
				lastNotifiedAt: pg.LastNotifiedAt,
				firstSeen:      pg.FirstSeen,
			}
		}
		if pg.IncidentKey == "" {
			continue
		}
		members := make(map[model.IncidentKey]bool)
		for _, key := range pg.Members {
			if _, live := e.state[key]; live {
				members[key] = false
			}
		}
		for _, key := range pg.Resolved {
			if _, live := e.state[key]; live {
				members[key] = true
			}
		}
		if len(members) == 0 {
			// Nothing left to wait on. Drop the flush state too, so a genuine
			// recurrence opens a fresh group rather than updating a thread
			// whose members are all gone.
			delete(e.groupFlushStates, pg.GroupKey)
			continue
		}
		e.groupResolveTrackers[pg.GroupKey] = &groupResolveTracker{
			groupIncKey: pg.IncidentKey,
			members:     members,
			totalCount:  pg.TotalCount,
			summary:     pg.Summary,
			reason:      pg.Reason,
			firstSeen:   pg.FirstSeen,
			lastSeen:    pg.LastSeen,
			severity:    pg.Severity,
		}
		restored++
	}
	if restored > 0 {
		klog.InfoS("restored smart groups from ConfigMap", "count", restored)
	}
}
