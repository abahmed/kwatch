package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/model"
)

// KeyAliases maps state keys written by older versions to their current key.
// It is also used when restoring provider threads.
type KeyAliases map[model.IncidentKey]model.IncidentKey

// ResolveKeyAlias follows a migration chain and protects callers from a
// malformed cyclic alias map.
func ResolveKeyAlias(
	aliases KeyAliases, key model.IncidentKey,
) model.IncidentKey {
	seen := map[model.IncidentKey]bool{}
	for key != "" && !seen[key] {
		seen[key] = true
		next, ok := aliases[key]
		if !ok || next == "" || next == key {
			break
		}
		key = next
	}
	return key
}

// RestoreIncidentRecords converts persisted incidents to canonical records,
// merges collisions, and returns aliases for dependent persisted state.
func RestoreIncidentRecords(
	records []model.PersistedIncident,
) (map[model.IncidentKey]*model.Incident, KeyAliases) {
	incidents := make(map[model.IncidentKey]*model.Incident, len(records))
	aliases := make(KeyAliases, len(records))
	for i := range records {
		inc := records[i].ToIncident()
		rawKey := inc.Key
		key := CanonicalIncidentKey(rawKey)
		aliases[rawKey] = key
		inc.Key = key
		inc.ID = incidentID(key)
		inc.Reason = normalizeReason(inc.Reason)
		inc.SuppressedBy = CanonicalIncidentKey(inc.SuppressedBy)
		if previous := incidents[key]; previous != nil {
			mergeRestoredIncident(previous, inc)
			continue
		}
		incidents[key] = inc
	}
	return incidents, aliases
}

func incidentStateRank(state model.IncidentState) int {
	switch state {
	case model.StateActive:
		return 3
	case model.StatePendingResolve:
		return 2
	case model.StateResolved:
		return 1
	default:
		return 0
	}
}

func mergeRestoredIncident(dst, src *model.Incident) {
	if incidentStateRank(src.State) > incidentStateRank(dst.State) {
		dst.State = src.State
		dst.ResolveAt = src.ResolveAt
	}
	dst.FirstSeen = earliest(dst.FirstSeen, src.FirstSeen)
	dst.LastSeen = latest(dst.LastSeen, src.LastSeen)
	dst.LastUpdate = latest(dst.LastUpdate, src.LastUpdate)
	if dst.Count < src.Count {
		dst.Count = src.Count
	}
	if dst.PeakResources < src.PeakResources {
		dst.PeakResources = src.PeakResources
	}
	for resource := range src.Resources {
		dst.Resources[resource] = true
	}
	for container := range src.Containers {
		dst.Containers[container] = true
	}
	if dst.Hint == "" {
		dst.Hint = src.Hint
	}
	if dst.Facts.IsZero() {
		dst.Facts = src.Facts
	}
	if dst.Name == "" {
		dst.Name = src.Name
	}
	if dst.Resource == "" {
		dst.Resource = src.Resource
	}
	if dst.OwnerKind == "" {
		dst.OwnerKind = src.OwnerKind
	}
	if dst.Severity == "" {
		dst.Severity = src.Severity
	}
	if dst.NotifiedSig == "" {
		dst.NotifiedSig = src.NotifiedSig
	}
	if src.LastNotifiedAt.After(dst.LastNotifiedAt) {
		dst.LastNotifiedAt = src.LastNotifiedAt
	}
	if dst.RenotifyCount < src.RenotifyCount {
		dst.RenotifyCount = src.RenotifyCount
	}
	if dst.SuppressedBy == "" {
		dst.SuppressedBy = src.SuppressedBy
	}
	if dst.Fingerprint == "" {
		dst.Fingerprint = src.Fingerprint
	}
	dst.ID = incidentID(dst.Key)
}

func earliest(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}
	return a
}

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
