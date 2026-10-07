package detectors

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxNamedUsers bounds how many pods a stuck claim names.
const maxNamedUsers = 3

// claimUsers names the pods that still mount a claim whose deletion is
// stuck: the claim-protection finalizer waits for them to go. The
// result is "pod db-0" or "pods a, b and 2 more"; empty when no pod
// is known to use it.
func claimUsers(ctx detection.Context, e inventory.Entity) string {
	if e.ID.Kind != kube.KindPVC || ctx.Model == nil {
		return ""
	}
	var names []string
	for _, id := range ctx.Model.Related(
		e.ID, inventory.Mounts, inventory.Incoming) {
		if id.Kind == kube.KindPod {
			names = append(names, id.Name)
		}
	}
	sort.Strings(names)
	switch {
	case len(names) == 0:
		return ""
	case len(names) == 1:
		return "pod " + names[0]
	case len(names) <= maxNamedUsers:
		return "pods " + strings.Join(names, ", ")
	}
	return "pods " + strings.Join(names[:maxNamedUsers], ", ") + " and " +
		countText(float64(len(names)-maxNamedUsers)) + " more"
}

// finalizerEvidence names the finalizers that hold a deletion, quoted.
func finalizerEvidence(e inventory.Entity) []detection.Evidence {
	finalizers := strings.Join(splitList(text(e, kube.AttrFinalizers)), ", ")
	if finalizers == "" {
		return nil
	}
	return []detection.Evidence{{Label: "finalizers", Value: finalizers}}
}
