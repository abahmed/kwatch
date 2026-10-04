package detectors

import (
	"sort"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ImageDrift detects a workload whose running pods pull the same image
// reference but run different builds of it: the tag was moved, and
// half the replicas picked up the new content without a rollout. It
// explains why some replicas fail and others do not.
type ImageDrift struct{}

// Name implements detection.Detector.
func (ImageDrift) Name() string { return "image-drift" }

// Kinds implements detection.Detector.
func (ImageDrift) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindDeployment, kube.KindStatefulSet,
		kube.KindDaemonSet}
}

// Detect implements detection.Detector.
func (ImageDrift) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if flag(e, kube.AttrDeleting) || !ctx.Synced(kube.KindPod) {
		return nil
	}
	builds := imageBuilds(ctx.Model, runningPodsOf(ctx.Model, e.ID))
	var out []detection.Finding
	for _, image := range sortedImages(builds) {
		ids := builds[image]
		if len(ids) < 2 {
			continue
		}
		out = append(out, detection.Finding{
			Reason:   reasons.ImageDigestDrift,
			Severity: detection.Warning,
			Since:    ctx.Onset("image-drift/"+image, ctx.Now),
			Summary: "Its pods run " + strconv.Itoa(len(ids)) +
				" different builds of image " + image + " under the same tag",
			Evidence: []detection.Evidence{
				{Label: "image", Value: image},
				{Label: "builds", Value: strings.Join(ids, ", ")},
			},
		})
	}
	return out
}

// imageBuilds maps each image reference the pods' containers declare to
// the distinct image IDs they actually run, sorted.
func imageBuilds(
	model inventory.Reader, pods []inventory.EntityID,
) map[string][]string {
	seen := map[string]map[string]bool{}
	for _, pod := range pods {
		for _, id := range model.Related(pod, inventory.PartOf,
			inventory.Incoming) {
			if container, ok := model.Entity(id); ok {
				noteBuild(seen, container)
			}
		}
	}
	out := map[string][]string{}
	for image, ids := range seen {
		for id := range ids {
			out[image] = append(out[image], id)
		}
		sort.Strings(out[image])
	}
	return out
}

// noteBuild records the image ID a container runs under its image
// reference, when the kubelet reported both.
func noteBuild(seen map[string]map[string]bool, container inventory.Entity) {
	image := text(container, kube.AttrImage)
	imageID := text(container, kube.AttrImageID)
	if image == "" || imageID == "" {
		return
	}
	if seen[image] == nil {
		seen[image] = map[string]bool{}
	}
	seen[image][imageID] = true
}

func sortedImages(builds map[string][]string) []string {
	out := make([]string, 0, len(builds))
	for image := range builds {
		out = append(out, image)
	}
	sort.Strings(out)
	return out
}
