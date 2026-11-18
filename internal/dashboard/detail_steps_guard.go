package dashboard

import (
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// rootStepText is the step that reads the root when no stored step is
// left about the incident.
const rootStepText = "See the object's state and events"

// goneStepText is the line shown when every stored step was dropped and
// the root itself no longer exists.
const goneStepText = "The object no longer exists in the cluster, so " +
	"there is nothing left to look at"

// currentSteps keeps the stored steps that are still about p. Steps are
// written with a message, and an incident's members change after it: a
// step that describes one object by name (kubectl describe KIND NAME)
// is kept only while that object is part of p (stepObjectsOf) and still
// exists in model (objectGone), so a step never points at an object
// another issue now holds or one that was deleted. When every step is
// dropped that way, the root is described instead, or, when the root is
// gone too, a line says so. Any other step is kept: it names no single
// object, or names one through the facts of the case (a gone node, the
// volume of an attachment). A nil model skips the existence check.
func currentSteps(
	steps []FixStep, p incident.Incident, model inventory.Reader,
) []FixStep {
	if len(steps) == 0 {
		return steps
	}
	known := stepObjectsOf(p)
	out := make([]FixStep, 0, len(steps))
	for _, s := range steps {
		if id, ok := describedObject(s.Command); ok &&
			(!known[id.plain()] || objectGone(model, id)) {
			continue
		}
		out = append(out, s)
	}
	if len(out) > 0 || p.Root.Name == "" ||
		(p.Root.Group == "" && !apiKind(lowerKind(p.Root))) {
		return out
	}
	if objectGone(model, stepObject{kind: string(lowerKind(p.Root)),
		group: p.Root.Group, namespace: p.Root.Namespace,
		name: p.Root.Name}) {
		return []FixStep{{Text: goneStepText}}
	}
	return []FixStep{{Text: rootStepText,
		Command: commandsFor(nil, p.Root)[0]}}
}

// objectGone reports that the model tracks the object's kind (it holds
// an object of it) and holds no such object: it was deleted. A kind the
// model holds nothing of may simply not be watched, so it is never
// judged.
func objectGone(model inventory.Reader, o stepObject) bool {
	if model == nil {
		return false
	}
	kind := inventory.Kind(o.kind)
	if len(model.Entities(kind)) == 0 {
		return false
	}
	return !model.Exists(inventory.EntityID{Kind: kind, Group: o.group,
		Namespace: o.namespace, Name: o.name})
}

// stepObject is an object as a step names it: kind, namespace and name,
// the kind in lower case. group is the API group the step spelled
// ("kind.group"), kept apart from the kind so steps compare with the
// incident's objects by plain.
type stepObject struct{ kind, group, namespace, name string }

// plain is o without its group, as stepObjectsOf keys objects.
func (o stepObject) plain() stepObject {
	o.group = ""
	return o
}

// stepObjectsOf are the objects p is about, as the affected list
// names them (affectedOf): its root, its cause, the objects merged into
// it, its failing members (a container as its pod) and what its
// failure reaches.
func stepObjectsOf(p incident.Incident) map[stepObject]bool {
	ids := []inventory.EntityID{p.Root, p.Subject}
	if c := p.Cause; c != nil {
		ids = append(ids, c.Root)
		ids = append(ids, c.Chain...)
		if c.Change != nil {
			ids = append(ids, c.Change.Entity)
		}
	}
	ids = append(ids, p.Merged...)
	ids = append(ids, p.Impact...)
	for _, f := range p.Members {
		if f.Advisory {
			continue
		}
		id := f.Entity
		if id.Kind == kube.KindContainer {
			id = podOfContainer(nil, id)
		}
		ids = append(ids, id)
	}
	out := make(map[stepObject]bool, len(ids))
	for _, id := range ids {
		out[stepObject{kind: string(lowerKind(id)),
			namespace: id.Namespace, name: id.Name}] = true
	}
	return out
}

// lowerKind is id's kind in lower case.
func lowerKind(id inventory.EntityID) inventory.Kind {
	return inventory.Kind(strings.ToLower(string(id.Kind)))
}

// describedObject reads the one object a "kubectl describe KIND NAME"
// command names, its namespace from -n or --namespace; ok is false for
// any other command, one that names several objects, KIND/NAME, or a
// kind by a short name.
func describedObject(command string) (stepObject, bool) {
	fields := strings.Fields(command)
	if len(fields) < 4 || fields[0] != "kubectl" {
		return stepObject{}, false
	}
	var out stepObject
	var args []string
	for i := 1; i < len(fields); i++ {
		switch f := fields[i]; {
		case (f == "-n" || f == "--namespace") && i+1 < len(fields):
			out.namespace = fields[i+1]
			i++
		case strings.HasPrefix(f, "--namespace="):
			out.namespace = strings.TrimPrefix(f, "--namespace=")
		case strings.HasPrefix(f, "-"):
			return stepObject{}, false
		default:
			args = append(args, f)
		}
	}
	if len(args) != 3 || args[0] != "describe" ||
		strings.ContainsAny(args[1]+args[2], "/|,'\"") {
		return stepObject{}, false
	}
	kind, group, _ := strings.Cut(args[1], ".")
	kind, group = strings.ToLower(kind), strings.ToLower(group)
	if group == "" && !apiKind(inventory.Kind(kind)) {
		// A short name ("pvc", "deploy") or a plural: not the form
		// a step names a member by, so not one to judge.
		return stepObject{}, false
	}
	out.kind, out.group, out.name = kind, group, args[2]
	return out, true
}
