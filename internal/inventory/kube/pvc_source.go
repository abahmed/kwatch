package kube

import corev1 "k8s.io/api/core/v1"

// AttrRestoreSnapshot is the name of the VolumeSnapshot, in the claim's
// own namespace, a claim is restored from. Set only on such a claim.
const AttrRestoreSnapshot = "restore.snapshot"

// snapshotAPIGroup is the API group of VolumeSnapshots.
const snapshotAPIGroup = "snapshot.storage.k8s.io"

// claimSnapshotSource is the VolumeSnapshot the claim's data source
// names, or "".
func claimSnapshotSource(pvc *corev1.PersistentVolumeClaim) string {
	if src := pvc.Spec.DataSource; src != nil && src.Kind == "VolumeSnapshot" &&
		src.APIGroup != nil && *src.APIGroup == snapshotAPIGroup {
		return src.Name
	}
	if src := pvc.Spec.DataSourceRef; src != nil &&
		src.Kind == "VolumeSnapshot" && src.APIGroup != nil &&
		*src.APIGroup == snapshotAPIGroup && src.Namespace == nil {
		return src.Name
	}
	return ""
}

// AttrSelectedNode is the node the scheduler picked for a claim that
// waits for its first consumer (the selected-node annotation). Set only
// when the annotation is present.
const AttrSelectedNode = "selected.node"

// selectedNodeAnnotations name the node chosen for a delayed-binding
// claim, the current key first.
var selectedNodeAnnotations = []string{
	"volume.kubernetes.io/selected-node",
	"volume.beta.kubernetes.io/selected-node",
}

// claimSelectedNode is the node the claim's annotations name, or "".
func claimSelectedNode(pvc *corev1.PersistentVolumeClaim) string {
	for _, key := range selectedNodeAnnotations {
		if name := pvc.Annotations[key]; name != "" {
			return name
		}
	}
	return ""
}
