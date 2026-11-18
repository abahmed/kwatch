package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// LinkDeploys links the release that deploys the effect to it: a Helm
// release and the workloads its marks name (inventory.DeployedBy).
const LinkDeploys LinkType = "deploys"

// helmReleaseModes are the modes of a release whose latest revision
// failed or hangs.
var helmReleaseModes = []detection.Mode{
	detection.ModeHelmReleaseFailed, detection.ModeHelmReleaseStuck}

// deployedFailures are the failures of a workload, or of its pods, that
// a release that went wrong can leave behind: pods that do not run,
// objects the controller cannot create and rollouts that stop.
var deployedFailures = append(append([]detection.Mode{
	detection.ModeRolloutStuck, detection.ModeUnavailable,
	detection.ModeStatefulSetRolloutStuck, detection.ModeJobFailed},
	podFailures...), configModes...)

// helmRows blame a Helm release whose latest revision failed or hangs
// for the failing workloads it deploys. Only a failure that began after
// the revision did is explained (outsideRelease), and the release stays
// a suspect unless the workloads' changes name it (releaseUnclaimed).
var helmRows = []Row{
	{
		Name:  RuleHelmReleaseFailed,
		Cause: Side{Kind: kube.KindHelmRelease, Modes: helmReleaseModes},
		Link:  LinkDeploys,
		Effect: Side{Kind: AnyKind, Modes: deployedFailures,
			NotModes: helmReleaseModes},
		// Above rollout (0.7): a revision Helm could not finish
		// explains the rollout it made, and undoing the release, not
		// the rollout alone, keeps Helm's history true.
		Prior: 0.8,
	},
}

// releaseHops lead from a workload to the release that deploys it, only
// while the release has failed or hangs: a healthy release is not
// walked, so it is never listed among what was checked.
func (v *view) releaseHops(id inventory.EntityID) []hop {
	var out []hop
	for _, release := range v.s.Model.Related(
		id, inventory.DeployedBy, inventory.Outgoing,
	) {
		if release.Kind != kube.KindHelmRelease ||
			release.Namespace != id.Namespace {
			continue
		}
		v.gate(id, release)
		if v.hasMode(release, helmReleaseModes) {
			out = append(out, hop{link: LinkDeploys, to: release})
		}
	}
	return out
}

// outsideRelease reports a failure that did not begin in the causal
// window of the revision the release hop blames. What already failed
// when the install, upgrade or rollback began was not broken by it and
// fails the same way without it; what began failing long after it did
// (a revision that failed months ago) has another cause.
func (v *view) outsideRelease(r reach, effect inventory.EntityID) bool {
	if r.link != LinkDeploys {
		return false
	}
	began := v.revisionBegan(r.id)
	start := v.earliestSince([]inventory.EntityID{effect})
	if !began.ok || !start.ok {
		return false
	}
	return start.t.Before(began.t.Add(-TemporalSlack)) ||
		start.t.After(began.t.Add(v.s.window()))
}

// revisionBegan is when the release's latest revision began: when Helm
// stored it, else when it got its status.
func (v *view) revisionBegan(release inventory.EntityID) timeValue {
	for _, name := range []string{
		kube.AttrHelmRevisionCreated, kube.AttrHelmStatusSince,
	} {
		a, ok := v.s.Model.Attribute(release, name)
		if t := a.Value.AsTime(); ok && !t.IsZero() {
			return timeValue{t: t, ok: true}
		}
	}
	return timeValue{}
}

// releaseUnclaimed reports a Helm release blamed for failures of which
// no workload's change names it as its app: the marks alone tie them.
func (v *view) releaseUnclaimed(c *candidate) bool {
	effects := c.others()
	if c.id.Kind != kube.KindHelmRelease || len(effects) == 0 {
		return false
	}
	for _, effect := range effects {
		unit := effect
		if pod, ok := v.podOf(effect); ok {
			unit = pod
		}
		for _, change := range v.changesOf(
			rootcause.TopOwner(v.s.Model, unit)) {
			if helmAppOf(change) == c.id {
				return false
			}
		}
	}
	return true
}

// helmAppOf is the Helm release a change's app names ("helm/<release>"
// or "helm/<namespace>/<release>"); the zero ID for any other app.
func helmAppOf(change inventory.Change) inventory.EntityID {
	tool, release, _ := strings.Cut(change.App, "/")
	if tool != "helm" || release == "" {
		return inventory.EntityID{}
	}
	space := change.Entity.Namespace
	if s, r, ok := strings.Cut(release, "/"); ok {
		space, release = s, r
	}
	return kube.HelmReleaseID(space, release)
}
