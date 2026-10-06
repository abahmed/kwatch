package detectors

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultImageDriftGrace is how long different builds of one image must
// keep running together before they count as drift. A rolling update of
// a mutable tag runs old and new builds side by side for a while; this
// wait starts when the newest build began running, so it ends well after
// the rollout does.
const DefaultImageDriftGrace = 5 * time.Minute

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
	if rollingOut(ctx.Model, e) {
		return nil
	}
	builds := imageBuilds(ctx.Model, runningPodsOf(ctx.Model, e.ID))
	var out []detection.Finding
	for _, image := range sortedImages(builds) {
		ids := builds[image].ids
		if len(ids) < 2 ||
			!sustained(ctx, "image-drift/"+image, builds[image].since,
				DefaultImageDriftGrace) {
			continue
		}
		out = append(out, detection.Finding{
			Reason:   reasons.ImageDigestDrift,
			Severity: detection.Warning,
			Since:    builds[image].since,
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

// rollingOut reports whether the workload is replacing pods right now:
// fewer replicas are updated than wanted, or the pods belong to more than
// one ReplicaSet (a Deployment's old and new set). Fewer available
// replicas alone is not a rollout: pods that fail because of the drifted
// build would then hide the drift for good. Replicas that are still
// starting after a rollout are covered by the grace period, which runs
// from when the newest build began.
func rollingOut(model inventory.Reader, e inventory.Entity) bool {
	desired, ok := number(e, kube.AttrReplicas)
	if daemon, found := number(e, kube.AttrDesiredReplicas); found {
		desired, ok = daemon, true
	}
	if ok {
		have, found := number(e, kube.AttrUpdatedReplicas)
		if found && have != desired {
			return true
		}
	}
	sets := 0
	for _, child := range model.Related(e.ID, inventory.OwnedBy,
		inventory.Incoming) {
		if child.Kind == kube.KindReplicaSet &&
			len(runningPodsOf(model, child)) > 0 {
			sets++
		}
	}
	return sets > 1
}

// imageBuild is the set of image IDs one image reference runs as, and
// when the newest of them started running.
type imageBuild struct {
	ids   []string
	since time.Time
}

// imageBuilds maps each image reference the pods' containers declare to
// the distinct image IDs they actually run, sorted.
func imageBuilds(
	model inventory.Reader, pods []inventory.EntityID,
) map[string]imageBuild {
	seen := map[string]map[string]bool{}
	started := map[string]time.Time{}
	for _, pod := range pods {
		for _, id := range model.Related(pod, inventory.PartOf,
			inventory.Incoming) {
			if container, ok := model.Entity(id); ok {
				noteBuild(seen, started, container)
			}
		}
	}
	out := map[string]imageBuild{}
	for image, ids := range seen {
		build := imageBuild{since: started[image]}
		for id := range ids {
			build.ids = append(build.ids, id)
		}
		sort.Strings(build.ids)
		out[image] = build
	}
	return out
}

// noteBuild records the image ID a container runs under its image
// reference, when the kubelet reported both, and the latest time one
// of those IDs was first seen.
func noteBuild(
	seen map[string]map[string]bool, started map[string]time.Time,
	container inventory.Entity,
) {
	image := text(container, kube.AttrImage)
	imageID := text(container, kube.AttrImageID)
	if image == "" || imageID == "" {
		return
	}
	if seen[image] == nil {
		seen[image] = map[string]bool{}
	}
	seen[image][imageID] = true
	started[image] = latest(started[image],
		valueSince(container, kube.AttrImageID))
}

func sortedImages(builds map[string]imageBuild) []string {
	out := make([]string, 0, len(builds))
	for image := range builds {
		out = append(out, image)
	}
	sort.Strings(out)
	return out
}
