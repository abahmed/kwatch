package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// storageRows cover what a claim depends on before any pod mounts it.
var storageRows = []Row{
	{
		// A claim names a StorageClass that does not exist, so nothing
		// provisions its volume. The claim is only the one who named
		// the class; the class is what is missing.
		Name: "storage-class-missing",
		Cause: Side{Kind: kube.KindStorageClass,
			Modes: []detection.Mode{ModeMissing}},
		Link: LinkUses,
		Effect: Side{Kind: kube.KindPVC,
			Modes: []detection.Mode{
				detection.ModeClaimFailed, detection.ModeVolume,
				detection.ModePending}},
		Prior: 0.8,
	},
	{
		// The same class, seen from the pod that waits for the claim:
		// the claim is only the way from the pod to the class.
		Name: "storage-class-missing-pending",
		Cause: Side{Kind: kube.KindStorageClass,
			Modes: []detection.Mode{ModeMissing}},
		Link: LinkUses,
		Effect: podSide(detection.ModePending, detection.ModeUnschedulable,
			detection.ModeCreating),
		Prior: 0.8,
	},
}
