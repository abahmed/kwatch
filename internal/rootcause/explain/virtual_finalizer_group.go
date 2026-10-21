package explain

import (
	"slices"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// stuckGroups memoises, per view, which stuck objects share their
// finalizers, which handlers all of them share and what the handler
// evidence reads from the cluster.
type stuckGroups struct {
	members  map[string][]inventory.EntityID
	handlers map[string][]inventory.EntityID
	// accounts holds, once built, the custom groups each ServiceAccount
	// may write; see accountGroups.
	accounts map[inventory.EntityID][]string
}

// groupKey names the objects held by the same finalizers in one
// namespace; empty when id has none a workload may run.
func (v *view) groupKey(id inventory.EntityID) string {
	names := v.finalizersOf(id)
	if len(names) == 0 {
		return ""
	}
	names = slices.Clone(names)
	sort.Strings(names)
	return id.Namespace + "/" + strings.Join(slices.Compact(names), ",")
}

// sharedHandlers are the handlers of every object of id's group: the
// objects are held by the same finalizers, so one answer serves them
// all and the group stays one problem.
func (v *view) sharedHandlers(id inventory.EntityID) []inventory.EntityID {
	key := v.groupKey(id)
	if key == "" {
		return nil
	}
	g := v.stuck()
	if shared, ok := g.handlers[key]; ok {
		return shared
	}
	members := g.members[key]
	if !containsID(members, id) {
		members = []inventory.EntityID{id}
	}
	shared := v.groupHandlers(members)
	g.handlers[key] = shared
	return shared
}

// stuck indexes the objects whose deletion is stuck, once.
func (v *view) stuck() *stuckGroups {
	if v.stuckIndex != nil {
		return v.stuckIndex
	}
	g := &stuckGroups{members: map[string][]inventory.EntityID{},
		handlers: map[string][]inventory.EntityID{}}
	v.stuckIndex = g
	for id, findings := range v.s.Findings {
		if !stuckDeleting(findings) {
			continue
		}
		if key := v.groupKey(id); key != "" {
			g.members[key] = append(g.members[key], id)
		}
	}
	for _, ids := range g.members {
		sortIDs(ids)
	}
	return g
}

func stuckDeleting(findings []detection.Finding) bool {
	for _, f := range findings {
		if f.Mode == detection.ModeStuckDeleting {
			return true
		}
	}
	return false
}
