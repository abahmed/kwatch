package dashboard

import (
	"cmp"
	"strings"
	"time"
)

// itemOrder compares two list items; negative puts a first.
type itemOrder func(a, b IncidentListItem) int

// sortField is one column the list can be sorted by: how it compares
// in ascending order and which direction it starts in.
type sortField struct {
	asc      func(now time.Time) itemOrder
	descends bool
}

// sortFields are the only sort keys /api/incidents accepts.
var sortFields = map[string]sortField{
	"severity":  {asc: fixed(bySeverity), descends: true},
	"title":     {asc: fixed(byTitle)},
	"namespace": {asc: fixed(byNamespace)},
	"object":    {asc: fixed(byObject)},
	"status":    {asc: fixed(byStatus)},
	"started":   {asc: fixed(byStarted), descends: true},
	"last":      {asc: fixed(byLast), descends: true},
	"duration":  {asc: byDuration, descends: true},
	"repeats":   {asc: fixed(byRepeats), descends: true},
	"acked":     {asc: fixed(byAcked)},
}

func fixed(o itemOrder) func(time.Time) itemOrder {
	return func(time.Time) itemOrder { return o }
}

// severityRank: higher is more urgent; quiet ("silent") is lowest.
var severityRank = map[string]int{"page": 3, "notify": 2, "digest": 1}

// statusRank: what needs a look first comes first.
var statusRank = map[string]int{"open": 0, "flapping": 1, "settling": 2,
	"recovering": 3, stateResolved: 4}

func bySeverity(a, b IncidentListItem) int {
	return cmp.Compare(severityRank[a.Severity], severityRank[b.Severity])
}

func byTitle(a, b IncidentListItem) int {
	return strings.Compare(a.Title, b.Title)
}

func byNamespace(a, b IncidentListItem) int {
	return strings.Compare(a.Namespace, b.Namespace)
}

func byObject(a, b IncidentListItem) int {
	if c := strings.Compare(a.WorkloadKind, b.WorkloadKind); c != 0 {
		return c
	}
	return strings.Compare(a.Workload, b.Workload)
}

func byStatus(a, b IncidentListItem) int {
	return cmp.Compare(statusRank[a.State], statusRank[b.State])
}

func byStarted(a, b IncidentListItem) int {
	return a.FirstSeen.Compare(b.FirstSeen)
}

// lastActivity is when a resolved issue resolved, else when an active
// one was last seen: the "Last seen or resolved" column.
func lastActivity(item IncidentListItem) time.Time {
	if !item.ResolvedAt.IsZero() {
		return item.ResolvedAt
	}
	return item.LastSeen
}

func byLast(a, b IncidentListItem) int {
	return lastActivity(a).Compare(lastActivity(b))
}

// byDuration compares how long issues lasted, or have lasted so far.
func byDuration(now time.Time) itemOrder {
	length := func(i IncidentListItem) time.Duration {
		end := now
		if !i.ResolvedAt.IsZero() {
			end = i.ResolvedAt
		}
		return end.Sub(i.FirstSeen)
	}
	return func(a, b IncidentListItem) int {
		return cmp.Compare(length(a), length(b))
	}
}

func byRepeats(a, b IncidentListItem) int {
	return cmp.Compare(a.RepeatCount, b.RepeatCount)
}

func byAcked(a, b IncidentListItem) int {
	return cmp.Compare(ackedRank(a), ackedRank(b))
}

func ackedRank(item IncidentListItem) int {
	if item.AckedBy != "" {
		return 1
	}
	return 0
}

// listOrder is the order of a list: the sort field in its direction,
// then the ID, so a page never depends on how the store listed them.
// Without a field it is the default of the list: the most severe first
// (urgent, needs attention, low, quiet), then newest started, on the
// active list (decision D4); latest activity first otherwise (latest
// resolved first on the resolved list, D5). Issues of one severity are
// always newest first, whichever way severity is sorted.
func listOrder(field string, desc bool, list string,
	now time.Time) itemOrder {
	if field == "" && list == stateActive {
		field, desc = "severity", true
	}
	if field == "" {
		field, desc = "last", true
	}
	primary := sortFields[field].asc(now)
	return thenID(func(a, b IncidentListItem) int {
		c := primary(a, b)
		if desc {
			c = -c
		}
		if c == 0 && field == "severity" {
			return b.FirstSeen.Compare(a.FirstSeen)
		}
		return c
	})
}

func thenID(o itemOrder) itemOrder {
	return func(a, b IncidentListItem) int {
		if c := o(a, b); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	}
}
