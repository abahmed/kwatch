package announce

import (
	"slices"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// A namespace outage is many workloads of one namespace failing at the
// same time with no cause tying them together. The announcer holds their
// announcements for a moment (see outage.go) so one message can name
// them all. The thresholds are in timings.go.

// isOutageCandidate reports an announcement that may belong to an
// outage: worth a message of its own and not explained by a cause, which
// would already tie it to the other failures.
func isOutageCandidate(d incident.Decision) bool {
	p := d.Incident
	return d.Action == incident.Announce && p.Tier >= incident.Notify &&
		p.Cause == nil && p.Root.Namespace != ""
}

// isOutage reports whether count incidents in a namespace of total
// workloads (zero when unknown) are an outage.
func isOutage(count, total int) bool {
	if count >= outageIncidents {
		return true
	}
	return count >= outageMinShare && total > 0 && count*2 >= total
}

// openedTogether keeps the incidents that opened within outageWindow of
// the newest one.
func openedTogether(ds []incident.Decision) []incident.Decision {
	var newest time.Time
	for _, d := range ds {
		if d.Incident.Opened.After(newest) {
			newest = d.Incident.Opened
		}
	}
	var out []incident.Decision
	for _, d := range ds {
		if !d.Incident.Opened.Before(newest.Add(-outageWindow)) {
			out = append(out, d)
		}
	}
	return out
}

// workloadCount is how many workloads the namespace has; zero without a
// model.
func (c *Collector) workloadCount(namespace string) int {
	if c.env.History == nil {
		return 0
	}
	n := 0
	for _, kind := range kube.WorkloadKinds {
		n += len(c.env.History.EntitiesIn(kind, namespace))
	}
	return n
}

// sharedBy finds what every member has in common: the node its pods run
// on, and a recent change in the namespace. Both are shown as facts.
func (c *Collector) sharedBy(
	members []incident.Decision, now time.Time,
) compose.OutageShared {
	var shared compose.OutageShared
	var nodes, changes [][]string
	byName := map[string]inventory.Change{}
	for _, d := range members {
		nodes = append(nodes, c.nodesOf(d.Incident))
		var names []string
		for _, c := range c.env.WithChanges(d, now).Facts.Changes {
			if c.At.After(d.Incident.Opened) {
				continue // not before the failure
			}
			names = append(names, c.Entity.String())
			byName[c.Entity.String()] = c
		}
		changes = append(changes, names)
	}
	if common := intersect(nodes); len(common) > 0 {
		shared.Node = common[0]
	}
	if common := intersect(changes); len(common) > 0 {
		change := byName[common[0]]
		shared.Change = &change
	}
	return shared
}

// nodesOf lists the nodes the pods in an incident's findings run on.
func (c *Collector) nodesOf(p incident.Incident) []string {
	if c.env.History == nil {
		return nil
	}
	var out []string
	for _, f := range p.Members {
		if f.Entity.Kind != kube.KindPod {
			continue
		}
		for _, n := range c.env.History.Related(
			f.Entity, inventory.RunsOn, inventory.Outgoing) {
			out = append(out, n.Name)
		}
	}
	return out
}

// intersect returns the sorted names every list has; none for no lists
// or when any list is empty.
func intersect(lists [][]string) []string {
	if len(lists) == 0 {
		return nil
	}
	common := slices.Clone(lists[0])
	for _, list := range lists[1:] {
		common = slices.DeleteFunc(common, func(s string) bool {
			return !slices.Contains(list, s)
		})
	}
	sort.Strings(common)
	return slices.Compact(common)
}
