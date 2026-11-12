package detectors

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// pullQPSExceeded is the kubelet's answer when its own pull rate limit
// (registryPullQPS, registryBurst) holds a pull back. The registry was
// never asked; the kubelet retries the pull on its own.
const pullQPSExceeded = "pull qps exceeded"

// pullNoteWindow is how far back a failed pull event explains a
// container that backs off pulling its image.
const pullNoteWindow = 10 * time.Minute

// pullThrottled reports a container that cannot pull its image only
// because its node's kubelet holds the pull back under its own rate
// limit: its waiting message says so, or, while it backs off, the
// latest failed pull event of it or its pod does.
func pullThrottled(ctx detection.Context, e inventory.Entity) bool {
	reason := text(e, kube.AttrStateReason)
	if reason != reasons.ErrImagePull && reason != reasons.ImagePullBackOff {
		return false
	}
	if strings.Contains(strings.ToLower(text(e, kube.AttrMessage)),
		pullQPSExceeded) {
		return true
	}
	if reason != reasons.ImagePullBackOff || ctx.Model == nil {
		return false
	}
	ids := []inventory.EntityID{e.ID}
	if pod, ok := owningPod(ctx, e); ok {
		ids = append(ids, pod.ID)
	}
	var latest inventory.Note
	for _, id := range ids {
		for _, note := range ctx.Model.Notes(id,
			ctx.Now.Add(-pullNoteWindow)) {
			if strings.HasPrefix(note.Message, "Failed to pull image") &&
				!note.At.Before(latest.At) {
				latest = note
			}
		}
	}
	return strings.Contains(strings.ToLower(latest.Message),
		pullQPSExceeded)
}

// throttledPull lowers a failed pull the kubelet held back under its
// pull rate limit to a warning: nothing is wrong with the image or the
// registry, and the kubelet retries it.
func throttledPull(s *detection.Finding, e inventory.Entity) {
	s.Severity = detection.Warning
	s.Summary = containerRole(e) + " is waiting to pull its image: " +
		"the kubelet on its node limits how fast it pulls images " +
		"and retries"
}
