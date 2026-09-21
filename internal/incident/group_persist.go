package incident

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
	canonical := make(map[string]model.PersistedGroup, len(groups))
	for _, source := range groups {
		pg := source
		if pg.GroupKey == "" {
			continue
		}
		pg.GroupKey = CanonicalGroupKey(pg.GroupKey)
		pg.IncidentKey = CanonicalIncidentKey(pg.IncidentKey)
		pg.Reason = normalizeReason(pg.Reason)
		if previous, ok := canonical[pg.GroupKey]; ok {
			mergePersistedGroup(&previous, pg)
			canonical[pg.GroupKey] = previous
			continue
		}
		canonical[pg.GroupKey] = pg
	}
	for _, value := range canonical {
		pg := &value
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
			key = CanonicalIncidentKey(key)
			if _, live := e.state[key]; live {
				members[key] = false
			}
		}
		for _, key := range pg.Resolved {
			key = CanonicalIncidentKey(key)
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

func mergePersistedGroup(dst *model.PersistedGroup, src model.PersistedGroup) {
	if dst.IncidentKey == "" {
		dst.IncidentKey = src.IncidentKey
	}
	if dst.Reason == "" {
		dst.Reason = src.Reason
	}
	if dst.Summary == "" {
		dst.Summary = src.Summary
	}
	if dst.TotalCount < src.TotalCount {
		dst.TotalCount = src.TotalCount
	}
	if dst.FirstSeen.IsZero() || (!src.FirstSeen.IsZero() &&
		src.FirstSeen.Before(dst.FirstSeen)) {
		dst.FirstSeen = src.FirstSeen
	}
	if src.LastSeen.After(dst.LastSeen) {
		dst.LastSeen = src.LastSeen
	}
	if src.LastNotifiedAt.After(dst.LastNotifiedAt) {
		dst.LastNotifiedAt = src.LastNotifiedAt
	}
	dst.Notified = dst.Notified || src.Notified
	if dst.Severity.Rank() < src.Severity.Rank() {
		dst.Severity = src.Severity
	}
	active := make(map[model.IncidentKey]bool, len(dst.Members))
	for _, key := range dst.Members {
		active[CanonicalIncidentKey(key)] = true
	}
	for _, key := range src.Members {
		active[CanonicalIncidentKey(key)] = true
	}
	resolved := make(map[model.IncidentKey]bool, len(dst.Resolved))
	for _, key := range dst.Resolved {
		resolved[CanonicalIncidentKey(key)] = true
	}
	for _, key := range src.Resolved {
		if !active[CanonicalIncidentKey(key)] {
			resolved[CanonicalIncidentKey(key)] = true
		}
	}
	dst.Members = dst.Members[:0]
	for key := range active {
		dst.Members = append(dst.Members, key)
	}
	dst.Resolved = dst.Resolved[:0]
	for key := range resolved {
		if !active[key] {
			dst.Resolved = append(dst.Resolved, key)
		}
	}
}
