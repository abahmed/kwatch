package compose

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// orphanFinalizerSteps end the deletions a finalizer of a removed
// controller holds: no controller in the cluster handles the finalizer
// (explain.ModeFinalizerUnhandled, not one known to run), so nothing
// will ever remove it. Each held object, up to maxNamedHeld, gets a
// read of its finalizers, then a patch that removes only that finalizer
// where it was seen, labelled as a change. The patch is offered only
// for a cause at least likely (undoSure); nil otherwise, and for any
// other cause.
func orphanFinalizerSteps(
	p incident.Incident, members []detection.Finding,
) []notification.Step {
	cause := p.Cause
	if cause == nil || cause.Rule != explain.RuleFinalizerUnhandled ||
		cause.Mode != explain.ModeFinalizerUnhandled ||
		cause.Root.Kind != explain.KindFinalizer || !undoSure(cause) {
		return nil
	}
	var steps []notification.Step
	for _, m := range heldObjects(members) {
		at, ok := finalizerIndex(m, cause.Root.Name)
		if !ok {
			continue
		}
		steps = append(steps, finalizerPatchSteps(m.Entity,
			cause.Root.Name, at)...)
		if len(steps) >= 2*maxNamedHeld {
			break
		}
	}
	return steps
}

// finalizerIndex is where name stands in the held object's finalizers,
// as kwatch last saw them; false when the object is not held by name.
func finalizerIndex(m detection.Finding, name string) (int, bool) {
	at := 0
	for _, f := range strings.Split(
		evidence(m, detection.EvidenceFinalizers), ", ") {
		switch {
		case f == name:
			return at, true
		case f != "":
			at++
		}
	}
	return 0, false
}

// finalizerPatchSteps read the object's finalizers, then remove name
// from them. The JSON patch first tests that name is still at index
// at, where kwatch saw it, so a list another controller changed since
// is refused rather than rewritten: a merge patch would set the whole
// list from kwatch's copy and put back a finalizer just removed.
func finalizerPatchSteps(
	id inventory.EntityID, name string, at int,
) []notification.Step {
	ref := kubectlKind(id) + " " + quote(id.Name) +
		namespaceFlag(id.Namespace)
	path := "/metadata/finalizers/" + strconv.Itoa(at)
	patch, _ := json.Marshal([]map[string]string{
		{"op": "test", "path": path, "value": name},
		{"op": "remove", "path": path},
	})
	return []notification.Step{{
		Text: "See the finalizers that hold " + shortName(id),
		Command: "kubectl get " + ref +
			" -o jsonpath='{.metadata.finalizers}'",
	}, {
		Text: "Remove finalizer " + name + " from " + shortName(id) +
			": no controller in the cluster handles it, so nothing " +
			"else removes it; if the patch is refused, the list " +
			"changed, so read it again (changes the cluster)",
		Command: "kubectl patch " + ref + " --type=json -p " +
			quote(string(patch)),
		Mutating: true,
	}}
}

// kubectlKind names a kind for kubectl: "secret", or "widget.example.com"
// for a kind of an API group, so a short name another group also uses
// cannot pick the wrong resource.
func kubectlKind(id inventory.EntityID) string {
	if id.Group == "" {
		return string(id.Kind)
	}
	return string(id.Kind) + "." + id.Group
}
