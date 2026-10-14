package scenarios

import (
	"sort"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/replay"
)

// alertKey is the identity paging tools deduplicate on.
func alertKey(key, dedup string) string {
	if dedup != "" {
		return dedup
	}
	return key
}

// pagesRaised counts the paging alerts a replay opened: a critical
// message of its own, whose alert was not already open. A roll-up only
// lists incidents and never pages, and a resolve closes its alert.
func pagesRaised(result replay.Result) int {
	open := map[string]bool{}
	pages := 0
	for _, m := range result.Messages {
		key := alertKey(m.Key, m.DedupKey)
		switch {
		case m.Resolved():
			delete(open, key)
		case m.IsPage() && len(m.Members) == 0 && !open[key]:
			open[key] = true
			pages++
		}
	}
	return pages
}

// pageLimit is the pages a scenario may raise: its MaxPages, or one for
// each outage it labels.
func pageLimit(e expectation) int {
	if e.MaxPages > 0 {
		return e.MaxPages
	}
	return len(e.roots())
}

// memberKeys lists the keys of the members that fail, sorted.
func memberKeys(p incident.Incident) []detection.Key {
	var keys []detection.Key
	for key, f := range p.Members {
		if !f.Advisory {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Entity.String()+keys[i].Reason <
			keys[j].Entity.String()+keys[j].Reason
	})
	return keys
}

// falseRecoveries counts the resolves that said an incident recovered
// while a failure it explained was still failing in another incident.
// A move to the incident that took its failures over is not one.
func falseRecoveries(result replay.Result) int {
	latest := map[string]incident.Incident{}
	told := map[string]map[detection.Key]bool{}
	count := 0
	for _, d := range result.Decisions {
		p := d.Incident
		if told[p.ID] == nil {
			told[p.ID] = map[detection.Key]bool{}
		}
		for _, key := range memberKeys(p) {
			told[p.ID][key] = true
		}
		if d.Action == incident.Resolve && p.SupersededBy == "" &&
			stillFailing(latest, p.ID, told[p.ID]) {
			count++
		}
		latest[p.ID] = p
	}
	return count
}

// stillFailing reports a member of the incident that another live
// incident holds now.
func stillFailing(
	latest map[string]incident.Incident, id string,
	members map[detection.Key]bool,
) bool {
	for other, p := range latest {
		if other == id || p.State == incident.Resolved {
			continue
		}
		for key := range p.Members {
			if members[key] && !p.Members[key].Advisory {
				return true
			}
		}
	}
	return false
}

// leftOpen counts the announced incidents still Open at the end of the
// replay with no member: their failures were folded elsewhere and nobody
// closed them.
func leftOpen(result replay.Result) int {
	count := 0
	for _, p := range result.Incidents {
		if p.State == incident.Open && !p.Announced.IsZero() &&
			len(p.Members) == 0 && p.SupersededBy == "" {
			count++
		}
	}
	return count
}
