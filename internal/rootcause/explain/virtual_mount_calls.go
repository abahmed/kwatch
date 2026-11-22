package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// mountFailures are the modes of a pod that waits on a volume it cannot
// mount.
var mountFailures = []detection.Mode{modeFailedMount,
	detection.ModeCreating}

// mountCall is the in-cluster Service that serves a network volume the
// failing pod mounts: the PersistentVolume names its NFS server by host,
// and the host is a Service name or the address of one. The pod calls
// that Service at mount time like a client would, so the same rows
// explain it.
func (v *view) mountCall(pod inventory.EntityID) (clusterCall, bool) {
	if len(v.unitFindingsIn(pod, mountFailures)) == 0 {
		return clusterCall{}, false
	}
	for _, claim := range v.s.Model.Related(
		pod, inventory.Mounts, inventory.Outgoing,
	) {
		v.gate(pod, claim)
		for _, volume := range v.s.Model.Related(
			claim, inventory.References, inventory.Outgoing,
		) {
			if call, ok := v.volumeServerCall(volume, pod); ok {
				return call, true
			}
		}
	}
	return clusterCall{}, false
}

// volumeServerCall is the Service named by a volume's NFS server host.
func (v *view) volumeServerCall(
	volume, pod inventory.EntityID,
) (clusterCall, bool) {
	e, ok := v.s.Model.Entity(volume)
	if !ok || volume.Kind != kube.KindPV {
		return clusterCall{}, false
	}
	host := attrText(e, kube.AttrNFSServer)
	if host == "" {
		return clusterCall{}, false
	}
	call, ok := v.serviceNamed([]string{"", host}, pod)
	if !ok {
		return clusterCall{}, false
	}
	call.class, call.line = endpointUnreachable, "mount of "+host
	call.mount = true
	return call, true
}

// callerOrMount are the modes of a pod that fails because a call failed
// or because the Service that serves its volume did not answer.
var callerOrMount = callerFailuresAnd(mountFailures...)
