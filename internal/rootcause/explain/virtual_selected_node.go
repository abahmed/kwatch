package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// RuleClaimSelectedNodeGone names the row that blames a claim whose
// selected node no longer exists.
const RuleClaimSelectedNodeGone = "claim-selected-node-gone"

// ModeSelectedNodeGone is a claim that still waits on the node the
// scheduler picked for it, which is not in the cluster any more.
const ModeSelectedNodeGone detection.Mode = "SelectedNodeGone"

// selectedNodeRows blame the claim for the pods that wait for it: the
// provisioner never binds a volume on a node that is gone.
var selectedNodeRows = []Row{{
	Name: RuleClaimSelectedNodeGone,
	Cause: Side{Kind: kube.KindPVC,
		Modes: []detection.Mode{ModeSelectedNodeGone}},
	Link: LinkMounts,
	Effect: podSide(detection.ModePending, detection.ModeUnschedulable,
		detection.ModeCreating),
	Prior: 0.8,
}}

// selectedNodeGone reports an unbound claim whose selected node is
// not in the model, when nodes are observed.
func (v *view) selectedNodeGone(claim inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(claim)
	if !ok || !v.s.synced(kube.KindNode) ||
		attrText(e, kube.AttrPhase) == claimBound {
		return false
	}
	name := attrText(e, kube.AttrSelectedNode)
	return name != "" &&
		!v.s.Model.Exists(inventory.CoreID(kube.KindNode, "", name))
}
