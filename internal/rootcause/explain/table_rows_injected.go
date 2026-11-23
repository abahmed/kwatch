package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// LinkInjects is a mutating webhook adding what the pod runs.
const LinkInjects LinkType = "injects"

// RuleWebhookInjectsImage names the rule below.
const RuleWebhookInjectsImage = "webhook-injects-image"

// injectedImageRow blames a mutating webhook configuration that changed
// just before pods failed to pull an image their workload's template does
// not name: the webhook added the container. It outranks the registry,
// which only refuses what something asked it for.
var injectedImageRow = Row{
	Name: RuleWebhookInjectsImage,
	Cause: Side{Kind: kube.KindMutatingWebhook,
		Modes: []detection.Mode{ModeSpecChanged, ModeChanged}},
	Link:   LinkInjects,
	Effect: podSide(detection.ModeImagePull),
	Prior:  0.85,
}

// injectedImageHops lead from a pod that fails to pull an image its
// owner's template does not name to the mutating webhook configurations:
// only the webhook can have added it.
func (v *view) injectedImageHops(id inventory.EntityID) []hop {
	if id.Kind != kube.KindPod && id.Kind != kube.KindContainer ||
		!v.hasMode(id, []detection.Mode{detection.ModeImagePull}) {
		return nil
	}
	pod, ok := v.podOf(id)
	if !ok || !v.pullsUntemplatedImage(pod) {
		return nil
	}
	v.gate(id, scanOf(kube.KindMutatingWebhook))
	var out []hop
	for _, hook := range v.s.Model.Entities(kube.KindMutatingWebhook) {
		v.gate(id, hook)
		out = append(out, hop{link: LinkInjects, to: hook})
	}
	return out
}

// pullsUntemplatedImage reports a pod with an image that is in no
// template image of its owning workload.
func (v *view) pullsUntemplatedImage(pod inventory.EntityID) bool {
	owner := rootcause.TopOwner(v.s.Model, pod)
	if owner == pod {
		return false
	}
	e, ok := v.s.Model.Entity(owner)
	if !ok {
		return false
	}
	known := map[string]bool{}
	for _, image := range strings.Split(
		attrText(e, kube.AttrTemplateImages), "\n") {
		known[image] = true
	}
	if len(known) == 1 && known[""] {
		return false // the template's images were not read
	}
	for _, image := range v.s.Model.Related(pod, inventory.Pulls,
		inventory.Outgoing) {
		if !known[image.Name] {
			return true
		}
	}
	return false
}
