package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// ModeNoPullSecret is a workload whose pods name no image pull Secret
// while other healthy workloads pull from the same registry with one:
// the registry accepts logins, and this workload sends none. It is a
// pseudo mode on the LinkSelf side of a row, like ModeMemoryTooLow.
const ModeNoPullSecret detection.Mode = "Config.NoPullSecret"

// pullWithoutSecretRow blames the workload that pulls anonymously where
// its neighbours log in.
var pullWithoutSecretRow = Row{
	Name: "pull-without-secret",
	Cause: Side{Group: AnyGroup, Kind: AnyKind,
		Modes: []detection.Mode{ModeNoPullSecret}},
	Link:   LinkSelf,
	Effect: podSide(detection.ModeImagePull),
	Prior:  0.75, Inside: true,
}

// pullConfigModes are the configuration faults of a pull that was
// refused for want of a login.
func (v *view) pullConfigModes(effect inventory.EntityID) []detection.Mode {
	if !v.hasMode(effect, []detection.Mode{detection.ModeImagePull}) ||
		!v.anonymousBesideLogins(effect) {
		return nil
	}
	return []detection.Mode{ModeNoPullSecret}
}

// anonymousBesideLogins reports a refused pull (an authentication
// answer) by a pod that names no pull Secret, from a registry that
// healthy pods of other workloads pull from with one. Healthy peers
// alone prove nothing: an expired token breaks only new pulls, and the
// peers pulled before it expired. A pod that sends no login is a
// different fact.
func (v *view) anonymousBesideLogins(effect inventory.EntityID) bool {
	text, ok := v.pullText(effect)
	pod, found := v.podOf(effect)
	if !ok || !found || classifyRegistryPull(text) != pullAuth {
		return false
	}
	e, ok := v.s.Model.Entity(pod)
	if !ok || len(kube.PullSecretNames(attrText(e,
		kube.AttrPullSecrets))) > 0 {
		return false
	}
	own := rootcause.TopOwner(v.s.Model, pod)
	for _, image := range v.s.Model.Related(pod, inventory.Pulls,
		inventory.Outgoing) {
		for _, peer := range v.loggedIn(rootcause.RegistryHost(
			image.Name)) {
			if peer != own {
				return true
			}
		}
	}
	return false
}

// loggedIn lists the workloads with a healthy pod that pulls from the
// registry host with an image pull Secret. The first call reads every
// pod once and keeps the answer for all hosts.
func (v *view) loggedIn(host string) []inventory.EntityID {
	if v.logins == nil {
		v.logins = v.readLogins()
	}
	return v.logins[host]
}

// readLogins maps each registry host to the workloads that log in to it
// with a healthy pod.
func (v *view) readLogins() map[string][]inventory.EntityID {
	seen := map[string]map[inventory.EntityID]bool{}
	out := map[string][]inventory.EntityID{}
	for _, pod := range v.s.Model.Entities(kube.KindPod) {
		if !v.namesPullSecret(pod) || !v.healthyPod(pod) {
			continue
		}
		top := rootcause.TopOwner(v.s.Model, pod)
		for _, image := range v.s.Model.Related(pod, inventory.Pulls,
			inventory.Outgoing) {
			host := rootcause.RegistryHost(image.Name)
			if seen[host] == nil {
				seen[host] = map[inventory.EntityID]bool{}
			}
			if !seen[host][top] {
				seen[host][top] = true
				out[host] = append(out[host], top)
			}
		}
	}
	return out
}

// namesPullSecret reports a pod that lists an image pull Secret.
func (v *view) namesPullSecret(pod inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(pod)
	return ok && len(kube.PullSecretNames(attrText(e,
		kube.AttrPullSecrets))) > 0
}
