package dashboard

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// maxAffected bounds the affected objects listed; maxLogCommands the
// "logs --previous" commands one pod gets.
const (
	maxAffected    = 20
	maxLogCommands = 3
)

// Affected statuses. The object the likely cause blames (a changed
// ConfigMap), else a root that does not fail itself (a node), is the
// likely cause.
const (
	statusFailing   = "failing"
	statusCause     = "cause"
	statusAffected  = "affected"
	statusRecovered = "recovered"
	statusGone      = "gone"
)

// notKubectl are the kinds kwatch models that are not API objects, so
// kubectl cannot describe them.
var notKubectl = map[inventory.Kind]bool{
	kube.KindImage: true, kube.KindRegistry: true, kube.KindZone: true,
	kube.KindNodePool: true, kube.KindExternalEndpoint: true,
	kube.KindContainer: true,
}

// affectedOf lists the objects p is about: its root, the other roots
// merged into it, the objects that fail (a container counts as its
// pod) and those its failure reaches. The root leads, then failing
// objects, longest failing first, then the rest by name. cut counts
// the objects left out. model may be nil.
func affectedOf(
	p incident.Incident, model inventory.Reader,
) ([]AffectedObject, int) {
	since := failingSince(p, model)
	ids := []inventory.EntityID{p.Root}
	if c, ok := causeObject(p); ok {
		ids = append(ids, c)
	}
	ids = append(ids, p.Merged...)
	for id := range since {
		ids = append(ids, id)
	}
	ids = append(ids, p.Impact...)
	var out []AffectedObject
	seen := map[inventory.EntityID]bool{}
	for _, id := range ids {
		if id.Name == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, affectedObject(p, model, id, since))
	}
	slices.SortFunc(out, compareAffected)
	if len(out) > maxAffected {
		return out[:maxAffected], len(out) - maxAffected
	}
	return out, 0
}

// failingSince maps each failing object to when it started failing,
// the earliest of its findings. A container's finding is its pod's.
func failingSince(
	p incident.Incident, model inventory.Reader,
) map[inventory.EntityID]time.Time {
	out := map[inventory.EntityID]time.Time{}
	for _, f := range p.Members {
		if f.Advisory {
			continue
		}
		id := f.Entity
		if id.Kind == kube.KindContainer {
			id = podOfContainer(model, id)
		}
		if at, ok := out[id]; !ok || (!f.Since.IsZero() &&
			(at.IsZero() || f.Since.Before(at))) {
			out[id] = f.Since
		}
	}
	return out
}

// podOfContainer is the pod a container is part of, read from its name
// ("pod/container") when the inventory does not know it.
func podOfContainer(
	model inventory.Reader, id inventory.EntityID,
) inventory.EntityID {
	if model != nil {
		if pod, ok := rootcause.PodOf(model, id); ok {
			return pod
		}
	}
	pod, _, _ := strings.Cut(id.Name, "/")
	return inventory.CoreID(kube.KindPod, id.Namespace, pod)
}

func affectedObject(
	p incident.Incident, model inventory.Reader, id inventory.EntityID,
	since map[inventory.EntityID]time.Time,
) AffectedObject {
	at, failing := since[id]
	status := statusAffected
	switch {
	case slices.Contains(p.Removed, id):
		status = statusGone
	case !p.Resolved.IsZero():
		status = statusRecovered
	case failing:
		status = statusFailing
	case id == causeID(p):
		status = statusCause
	}
	return AffectedObject{Kind: string(id.Kind), Namespace: id.Namespace,
		Name: id.Name, Status: status, Root: id == p.Root, Since: at,
		Commands: commandsFor(model, id)}
}

// causeObject is the object p's likely cause blames when it is not
// p's root: the object of the blamed change, else the cause's root.
func causeObject(p incident.Incident) (inventory.EntityID, bool) {
	if p.Cause == nil {
		return inventory.EntityID{}, false
	}
	id := p.Cause.Root
	if p.Cause.Change != nil && p.Cause.Change.Entity.Name != "" {
		id = p.Cause.Change.Entity
	}
	return id, id.Name != "" && id != p.Root
}

// causeID is the object labelled the likely cause: the one the cause
// blames, else p's root. Without a cause (no summary: the page says
// "Cause not found yet") no object is, so the zero ID.
func causeID(p incident.Incident) inventory.EntityID {
	if id, ok := causeObject(p); ok {
		return id
	}
	if causeText(p) == "" {
		return inventory.EntityID{}
	}
	return p.Root
}

// compareAffected puts the root first, then the likely cause, then
// failing objects, longest failing first, then everything else by kind
// and name.
func compareAffected(a, b AffectedObject) int {
	rank := func(o AffectedObject) int {
		switch {
		case o.Root:
			return 0
		case o.Status == statusCause:
			return 1
		case o.Status == statusFailing || !o.Since.IsZero():
			return 2
		}
		return 3
	}
	return cmp.Or(cmp.Compare(rank(a), rank(b)),
		a.Since.Compare(b.Since), cmp.Compare(a.Kind, b.Kind),
		cmp.Compare(a.Namespace, b.Namespace), cmp.Compare(a.Name, b.Name))
}

// commandsFor is "kubectl describe" for the object and, for a pod whose
// containers restarted, the logs of each one's previous run.
func commandsFor(
	model inventory.Reader, id inventory.EntityID,
) []string {
	if notKubectl[id.Kind] {
		return []string{}
	}
	out := []string{kubectl(id, "describe "+resourceName(id)+" "+
		id.Name)}
	if id.Kind != kube.KindPod || model == nil {
		return out
	}
	for _, c := range containerIDs(model, id) {
		if len(out) > maxLogCommands {
			break
		}
		restarts, ok := model.Attribute(c, kube.AttrRestarts)
		n, _ := restarts.Value.AsNumber()
		if !ok || n < 1 {
			continue
		}
		_, name, _ := strings.Cut(c.Name, "/")
		out = append(out, kubectl(id, "logs "+id.Name+" -c "+name+
			" --previous"))
	}
	return out
}

// kubectl prefixes args with the namespace of id, when it has one.
func kubectl(id inventory.EntityID, args string) string {
	if id.Namespace == "" {
		return "kubectl " + args
	}
	return "kubectl -n " + id.Namespace + " " + args
}

// resourceName is the kind as kubectl takes it: lower case, with the
// API group for objects outside the core groups.
func resourceName(id inventory.EntityID) string {
	kind := strings.ToLower(string(id.Kind))
	if id.Group == "" {
		return kind
	}
	return kind + "." + id.Group
}
