package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// attacherRows blame the workload that serves the driver whose
// attacher the pods' volumes wait for, once it has no ready pod.
var attacherRows = []Row{{
	Name: RuleAttacherDown,
	Cause: Side{Group: AnyGroup, Kind: AnyKind,
		Modes: []detection.Mode{ModeProvisionerDown}},
	Link:   LinkProvisions,
	Effect: podSide(modeFailedAttach),
	Prior:  0.8,
}}

// attachWaitPattern is the kubelet's wait for a CSI driver's attacher:
// group 1 is the driver name, in lower-case text.
var attachWaitPattern = regexp.MustCompile(
	`external-attacher of ([a-z0-9][a-z0-9.-]*) csi driver`)

// attacherHops lead from a pod whose volume attach timed out waiting
// for a driver's external attacher to the workloads that serve that
// driver. The same row as a claim's provisioner blames them, once none
// of them has a ready pod.
func (v *view) attacherHops(pod inventory.EntityID) []hop {
	if pod.Kind != kube.KindPod {
		return nil
	}
	name := firstNameIn(strings.ToLower(v.text(pod)), attachWaitPattern)
	if name == "" {
		return nil
	}
	return hopsTo(LinkProvisions, v.blameable(
		kube.ProvisionerWorkloads(v.s.Model, name))...)
}
