package upstream

import "slices"

// PodPhases are the values of pod.status.phase (k8s.io/api core/v1
// PodPhase constants).
var PodPhases = []string{
	"Pending", "Running", "Succeeded", "Failed", "Unknown",
}

// RuntimeTerminatedReasons are terminated reasons the container runtime
// reports in its CRI status (containerd and CRI-O); they are not in the
// Kubernetes source, so they are written here and not generated. The
// kubelet's own are KubeletTerminatedReasons.
var RuntimeTerminatedReasons = []string{"Error", "StartError"}

// TerminatedReasons are the reasons of a container in the terminated
// state, from the kubelet and from the runtime.
var TerminatedReasons = slices.Concat(
	KubeletTerminatedReasons, RuntimeTerminatedReasons)
