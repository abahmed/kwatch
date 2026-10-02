package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// LinkAuthorizes is the link from a failure to an RBAC object that
// grants (or stopped granting) the failing ServiceAccount its
// permissions.
const LinkAuthorizes LinkType = "authorizes"

// rbacKinds are the built-in RBAC kinds. kwatch watches their metadata
// only, so it sees that one was created, edited or deleted, not which
// subjects it names.
var rbacKinds = []inventory.Kind{
	"role", "rolebinding", "clusterrole", "clusterrolebinding",
}

// accessRows cover RBAC (ADR 0010 rule 13): an RBAC object changed just
// before the pods of its namespace failed with "forbidden" errors.
var accessRows = []Row{
	{
		Name: "rbac-change",
		Cause: Side{Kind: AnyKind,
			Modes: []detection.Mode{ModeChanged}},
		Link: LinkAuthorizes,
		Effect: Side{Kind: kube.KindPod, Signal: SignalForbidden,
			Modes: append([]detection.Mode{
				detection.ModeFailed, detection.ModeError}, podFailures...)},
		Prior: 0.75,
	},
}

// recentChangeSets is the part of the inventory that lists every recent
// change set, deleted objects included. The production model has it;
// small test models may not.
type recentChangeSets interface {
	RecentChangeSets(since time.Time) []inventory.ChangeSet
}

// accessHops lead from a failing pod to the RBAC objects of its
// namespace, and the cluster-wide ones, that changed in the window. The
// row then asks for a "forbidden" error before it blames one.
func (v *view) accessHops(pod inventory.EntityID) []hop {
	if !v.unitGate(pod) {
		return nil
	}
	var out []hop
	for _, id := range v.changedRBAC() {
		if id.Namespace == "" || id.Namespace == pod.Namespace {
			out = append(out, hop{link: LinkAuthorizes, to: id})
		}
	}
	return out
}

// changedRBAC lists the RBAC objects with a change in the window,
// present or deleted, once per solve.
func (v *view) changedRBAC() []inventory.EntityID {
	if v.rbac != nil {
		return *v.rbac
	}
	found := map[inventory.EntityID]bool{}
	for _, id := range v.rbacCandidates() {
		if len(v.changesOf(id)) > 0 {
			found[id] = true
		}
	}
	out := sortedKeys(found)
	v.rbac = &out
	return out
}

// rbacCandidates lists the present RBAC objects and the RBAC objects of
// recent change sets, which include deleted ones.
func (v *view) rbacCandidates() []inventory.EntityID {
	var out []inventory.EntityID
	for _, kind := range rbacKinds {
		out = append(out, v.s.Model.Entities(kind)...)
	}
	sets, ok := v.s.Model.(recentChangeSets)
	if !ok {
		return out
	}
	for _, set := range sets.RecentChangeSets(v.since) {
		for _, id := range set.Entities() {
			if isRBAC(id) {
				out = append(out, id)
			}
		}
	}
	return out
}

func isRBAC(id inventory.EntityID) bool {
	if id.Group != "" {
		return false
	}
	for _, kind := range rbacKinds {
		if id.Kind == kind {
			return true
		}
	}
	return false
}
