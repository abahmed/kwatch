package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// containerRows cover init and sidecar containers:
// a failing init container keeps the pod from starting, a failing
// sidecar (a mesh proxy, a secrets agent) keeps it from being ready.
// The blame is that container, not the application next to it. Pull
// and configuration errors are left out: they have outside causes
// (a registry, a missing Secret) that other rows name.
var containerRows = []Row{
	{
		Name: "helper-container-blocks-pod",
		Cause: Side{Kind: kube.KindContainer, Modes: []detection.Mode{
			detection.ModeCrashLoop, detection.ModeInitError,
			detection.ModeError, detection.ModeExit, detection.ModeOOMKilled,
			detection.ModeRestarting, detection.ModeProbe,
			detection.ModeNotReady}},
		Link: LinkBlocks,
		Effect: podSide(
			detection.ModeNotReady, detection.ModeInitializing,
			detection.ModePending, detection.ModeProbe, detection.ModeCrashLoop,
			detection.ModeRestarting, detection.ModeCreating),
		Prior: 0.8, Inside: true,
	},
	{
		// The workload's own tag moved under it: replicas running the
		// newer build fail while the others do not.
		Name: "image-drift",
		Cause: Side{Kind: AnyKind, Modes: []detection.Mode{
			detection.ModeImageDrift}},
		Link: LinkOwns,
		Effect: podSide(
			detection.ModeCrashLoop, detection.ModeError, detection.ModeExit,
			detection.ModeProbe, detection.ModeNotReady,
			detection.ModeRestarting),
		Prior: 0.7, Inside: true,
	},
}

// helperHops lead from a pod to its init and sidecar containers, which
// must succeed before the pod starts or is ready. The hop depends only
// on the containers' declared role, not on their health.
func (v *view) helperHops(pod inventory.EntityID) []hop {
	var out []hop
	for _, id := range v.s.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		container, ok := v.s.Model.Entity(id)
		if !ok || id.Kind != kube.KindContainer {
			continue
		}
		if attrBool(container, kube.AttrInit) ||
			attrBool(container, kube.AttrSidecar) {
			out = append(out, hop{link: LinkBlocks, to: id})
		}
	}
	return out
}

func attrBool(e inventory.Entity, name string) bool {
	attribute, ok := e.Attribute(name)
	if !ok {
		return false
	}
	value, _ := attribute.Value.AsBool()
	return value
}
