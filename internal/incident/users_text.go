package incident

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// routedUsers words a route to the failing pods: "Users are affected:
// route shop.example.com → api has 0 of 3 ready endpoints."
func routedUsers(r usersRoute) *Users {
	out := &Users{Route: r.address, Service: r.service.Name,
		Ready: r.count.ready, Total: r.count.total}
	words, counted := r.count.words()
	path := "route " + r.address + " → " + r.service.Name + " " + words +
		"."
	switch {
	case !counted:
		out.State, out.Text = UsersUnknown, "kwatch cannot tell whether "+
			"users are affected: "+path
	case r.count.ready == 0:
		out.State, out.Text = UsersAffected, "Users are affected: "+path
	default:
		out.State, out.Text = UsersServed, "Users are still served: "+path
	}
	return out
}

// unroutedUsers words Services no route kwatch sees leads to: the
// worst Service's endpoints, and who else calls the Services.
func unroutedUsers(
	model inventory.Reader, services []inventory.EntityID,
	workloads map[inventory.EntityID]bool,
) *Users {
	svc, count := services[0], serviceEndpoints(model, services[0])
	for _, id := range services[1:] {
		if c := serviceEndpoints(model, id); c.rank() < count.rank() {
			svc, count = id, c
		}
	}
	out := &Users{State: UsersNoRoute, Service: svc.Name,
		Ready: count.ready, Total: count.total}
	out.Callers, out.MoreCallers = callersOf(model, services, workloads)
	words, _ := count.words()
	text := "Service " + svc.Name + " " + words +
		"; kwatch does not see a route to it from outside the cluster"
	if !count.seen {
		text = "kwatch does not see Service " + svc.Name +
			" or a route to it from outside the cluster"
	}
	switch {
	case len(out.Callers) > 0:
		text += "; other workloads that call it: " +
			strings.Join(out.Callers, ", ")
		if out.MoreCallers > 0 {
			text += " and " + strconv.Itoa(out.MoreCallers) + " more"
		}
	default:
		text += " or another workload that calls it"
	}
	out.Text = text + "."
	return out
}

// callersOf names the other workloads whose pods call one of services,
// sorted, at most MaxCallers, and counts the rest.
func callersOf(
	model inventory.Reader, services []inventory.EntityID,
	workloads map[inventory.EntityID]bool,
) ([]string, int) {
	set := map[inventory.EntityID]bool{}
	for _, svc := range services {
		for _, id := range model.Related(svc, inventory.Calls,
			inventory.Incoming) {
			caller := id
			if pod, ok := rootcause.PodOf(model, id); ok {
				caller = rootcause.TopOwner(model, pod)
			}
			if !workloads[caller] && caller.Name != "" {
				set[caller] = true
			}
		}
	}
	var names []string
	for _, id := range sortedIDs(set) {
		if !containsName(names, id.Name) {
			names = append(names, id.Name)
		}
	}
	if len(names) > MaxCallers {
		return names[:MaxCallers], len(names) - MaxCallers
	}
	return names, 0
}

func containsName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}
