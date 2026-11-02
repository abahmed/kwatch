package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// recordWithheld counts the withheld findings of e. A readiness finding
// counts once per pod, and not at all when the pod already fails in a
// way Community reports: a container that crashes or never starts, or
// a node that is not ready, leaves the pod unready too, so the count
// would only repeat a problem people were already told about.
func (r restricted) recordWithheld(
	ctx detection.Context, e inventory.Entity,
	withheld, kept []detection.Finding,
) {
	if r.withheld == nil {
		return
	}
	for _, f := range withheld {
		category, ok := withheldCategory(f.Reason)
		if !ok {
			continue
		}
		key := e.ID.String() + "\x00" + f.Reason
		if category == feature.WithheldReadiness {
			pod, counted := readinessKey(ctx, e, kept)
			if !counted {
				continue
			}
			key = pod
		}
		r.withheld.Record(ctx.Now, category, key)
	}
}

// readinessKey is the key a withheld readiness finding of e counts
// under, its pod's; counted is false when the pod fails on its own or
// its node is not ready.
func readinessKey(
	ctx detection.Context, e inventory.Entity, kept []detection.Finding,
) (key string, counted bool) {
	if len(kept) > 0 {
		return "", false
	}
	pod := e
	if e.ID.Kind == kube.KindContainer {
		if owner, ok := owningPod(ctx, e); ok {
			pod = owner
		}
	}
	if podFailsOnItsOwn(ctx, pod) || nodeDown(ctx, pod) {
		return "", false
	}
	return pod.ID.String() + "\x00readiness", true
}

// podFailsOnItsOwn reports a pod with a container that is waiting or
// terminated with a reason, or that restarted recently: a crash loop or
// a start that never happens, which Community reports.
func podFailsOnItsOwn(ctx detection.Context, pod inventory.Entity) bool {
	if ctx.Model == nil {
		return false
	}
	for _, id := range ctx.Model.Related(
		pod.ID, inventory.PartOf, inventory.Incoming,
	) {
		c, ok := ctx.Model.Entity(id)
		if ok && c.ID.Kind == kube.KindContainer &&
			containerFailsOnItsOwn(ctx, c) {
			return true
		}
	}
	return false
}

// containerFailsOnItsOwn reports a container that is waiting or
// terminated with a reason other than a clean exit, or that is running
// again after a restart inside restartRecency.
func containerFailsOnItsOwn(
	ctx detection.Context, c inventory.Entity,
) bool {
	reason := text(c, kube.AttrStateReason)
	switch text(c, kube.AttrState) {
	case "waiting":
		return reason != ""
	case "terminated":
		code, _ := number(c, kube.AttrExitCode)
		return code != 0 || (reason != "" && reason != "Completed")
	case "running":
		restarts, _ := number(c, kube.AttrRestarts)
		last := timestamp(c, kube.AttrLastFinished)
		return restarts > 0 && !last.IsZero() &&
			ctx.Now.Sub(last) <= restartRecency
	}
	return false
}

// nodeDown reports a pod whose node is not ready: Community reports the
// node, and the pods on it are unready because of it.
func nodeDown(ctx detection.Context, pod inventory.Entity) bool {
	if ctx.Model == nil {
		return false
	}
	for _, id := range ctx.Model.Related(
		pod.ID, inventory.RunsOn, inventory.Outgoing,
	) {
		node, ok := ctx.Model.Entity(id)
		if !ok {
			continue
		}
		if status, _, _ := condition(node, "Ready"); status != "" &&
			status != "True" {
			return true
		}
	}
	return false
}
