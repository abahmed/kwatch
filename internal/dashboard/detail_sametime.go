package dashboard

import (
	"cmp"
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
)

// Bounds of "Happening at the same time": how far apart two starts
// may be, how many issues are listed and how many of the nearest are
// checked for a shared node (each reads the incident's pods).
const (
	sameTimeWindow    = 5 * time.Minute
	maxSameTime       = 8
	maxSameTimeNodeOf = 64
)

// Relations of another issue to this one, closest first.
const (
	relationNode      = "same node"
	relationNamespace = "same namespace"
	relationCluster   = "cluster"
)

// sameTimeOf lists the other incidents that started within
// sameTimeWindow of p, those on the same node first, then the same
// namespace, then the rest of the cluster, nearest start first; cut
// counts those left out. Active quiet issues are left out, as on every
// list.
func sameTimeOf(
	p incident.Incident, all []incident.Incident, model inventory.Reader,
) ([]SameTimeItem, int) {
	near := nearIncidents(p, all)
	if len(near) == 0 {
		return nil, 0
	}
	mine := nodeSet(model, p)
	items := make([]SameTimeItem, 0, len(near))
	for i, q := range near {
		rel := relationCluster
		switch {
		case i < maxSameTimeNodeOf && shareNode(mine, nodeSet(model, q)):
			rel = relationNode
		case p.Root.Namespace != "" &&
			q.Root.Namespace == p.Root.Namespace:
			rel = relationNamespace
		}
		items = append(items, sameTimeItem(q, rel))
	}
	slices.SortStableFunc(items, func(a, b SameTimeItem) int {
		return cmp.Compare(relationRank(a.Relation),
			relationRank(b.Relation))
	})
	if len(items) > maxSameTime {
		return items[:maxSameTime], len(items) - maxSameTime
	}
	return items, 0
}

// nearIncidents are the incidents other than p that opened within
// sameTimeWindow of it, nearest first, then by ID.
func nearIncidents(
	p incident.Incident, all []incident.Incident,
) []incident.Incident {
	var out []incident.Incident
	for i, q := range all {
		if i >= maxSimilarScan {
			break
		}
		if q.ID == p.ID || q.Opened.IsZero() ||
			(q.Tier == incident.Silent && q.State != incident.Resolved) {
			continue
		}
		if gap(p.Opened, q.Opened) <= sameTimeWindow {
			out = append(out, q)
		}
	}
	slices.SortFunc(out, func(a, b incident.Incident) int {
		return cmp.Or(cmp.Compare(gap(p.Opened, a.Opened),
			gap(p.Opened, b.Opened)), cmp.Compare(a.ID, b.ID))
	})
	return out
}

func gap(a, b time.Time) time.Duration {
	if a.After(b) {
		return a.Sub(b)
	}
	return b.Sub(a)
}

func relationRank(rel string) int {
	switch rel {
	case relationNode:
		return 0
	case relationNamespace:
		return 1
	}
	return 2
}

func sameTimeItem(q incident.Incident, rel string) SameTimeItem {
	return SameTimeItem{ID: q.ID, Title: titleFor(q, redact.Evidence),
		Severity: q.Tier.String(), State: q.State.String(),
		Namespace: q.Root.Namespace, StartedAt: q.Opened, Relation: rel}
}

// nodeSet is the nodes p involves: its root and merged roots when they
// are nodes, and the nodes its pods run on. Empty without a model.
func nodeSet(
	model inventory.Reader, p incident.Incident,
) map[inventory.EntityID]bool {
	out := map[inventory.EntityID]bool{}
	for _, id := range append([]inventory.EntityID{p.Root}, p.Merged...) {
		if id.Kind == kube.KindNode && id.Name != "" {
			out[id] = true
		}
	}
	if model == nil {
		return out
	}
	for _, pod := range podsOf(model, p) {
		for _, node := range model.Related(pod, inventory.RunsOn,
			inventory.Outgoing) {
			if node.Kind == kube.KindNode {
				out[node] = true
			}
		}
	}
	return out
}

func shareNode(a, b map[inventory.EntityID]bool) bool {
	for id := range a {
		if b[id] {
			return true
		}
	}
	return false
}
