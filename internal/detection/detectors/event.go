package detectors

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// EventWindow is how recent a Warning event must be to count as current.
const EventWindow = 15 * time.Minute

// eventReasons are Warning event reasons that are failures on their own
// and are not already detected from object state. Reasons that state
// already shows (BackOff, Unhealthy, FailedScheduling, Evicted, HPA and
// Job conditions) stay evidence only, so nothing is reported twice.
// Webhook failures surface as FailedCreate; cluster-autoscaler's
// NotTriggerScaleUp is a Normal event, which is never ingested.
var eventReasons = map[string]detection.Severity{
	"FailedMount":            detection.Critical,
	"FailedAttachVolume":     detection.Critical,
	"FailedCreatePodSandBox": detection.Critical,
	"FailedCreate":           detection.Critical,
	"ProvisioningFailed":     detection.Warning,
	"FailedBinding":          detection.Warning,
	"VolumeResizeFailed":     detection.Warning,
	"FailedDaemonPod":        detection.Warning,
	"NetworkNotReady":        detection.Critical,
	"FailedKillPod":          detection.Warning,
	"FailedPreStopHook":      detection.Warning,
	"FailedPostStartHook":    detection.Critical,
	// Kubelet node events (disk reclaim): disk reclaim and garbage collection.
	reasons.EvictionThresholdMet: detection.Warning,
	// A failed image garbage collection on its own is an early disk
	// signal, not an outage: it goes to the digest. DiskPressure, which
	// follows when the disk really fills, is detected from the node's
	// conditions and still notifies.
	reasons.ImageGCFailed:       detection.Info,
	reasons.FreeDiskSpaceFailed: detection.Info,
	reasons.ContainerGCFailed:   detection.Warning,
	// Volume and device events (volume mapping): kubelet block-volume mapping and
	// DRA driver preparation, both on the pod that waits for them.
	reasons.FailedMapVolume:               detection.Critical,
	reasons.FailedPrepareDynamicResources: detection.Critical,
}

// Event turns recent failure-shaped Warning events into findings for any
// entity kind.
type Event struct{}

// Name implements detection.Detector.
func (Event) Name() string { return "event" }

// Kinds implements detection.Detector.
func (Event) Kinds() []inventory.Kind {
	return []inventory.Kind{detection.AnyKind}
}

// Detect implements detection.Detector.
func (Event) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if deleted(ctx, e) {
		return nil
	}
	var order []string
	byReason := map[string]*eventGroup{}
	for _, note := range ctx.Model.Notes(e.ID, ctx.Now.Add(-EventWindow)) {
		severity, failing := eventReasons[note.Reason]
		if !note.Warning || !failing || overcomeAtStart(e, note) {
			continue
		}
		ctx.RecheckAfter(note.At.Add(EventWindow).Sub(ctx.Now))
		key := eventReason(note)
		group, seen := byReason[key]
		if !seen {
			group = &eventGroup{}
			byReason[key] = group
			order = append(order, key)
		}
		group.add(note, severity)
	}
	out := make([]detection.Finding, 0, len(order))
	for _, key := range order {
		out = append(out, byReason[key].finding(key))
	}
	return append(out, unusualEvents(ctx, e)...)
}

// eventGroup folds the notes of one reason: notes are kept per source,
// but findings are keyed by reason.
type eventGroup struct {
	newest   inventory.Note
	severity detection.Severity
	count    int
}

func (g *eventGroup) add(note inventory.Note, severity detection.Severity) {
	if note.At.After(g.newest.At) || g.newest.Reason == "" {
		g.newest = note
	}
	g.severity = max(g.severity, severity)
	g.count += max(note.Count, 1)
}

func (g *eventGroup) finding(reason string) detection.Finding {
	evidence := []detection.Evidence{
		{Label: "event", Value: g.newest.Message}}
	if g.count > 1 {
		evidence = append(evidence, detection.Evidence{
			Label: "occurrences", Value: strconv.Itoa(g.count),
		})
	}
	return detection.Finding{
		Reason: reason, Severity: g.severity, Since: g.newest.At,
		Mode:     eventMode(g.newest),
		Summary:  eventSummary(g.newest),
		Evidence: evidence,
	}
}

func eventSummary(note inventory.Note) string {
	if summary, ok := storageEventSummaries[eventReason(note)]; ok {
		return summary
	}
	switch note.Reason {
	case "FailedMount", "FailedAttachVolume":
		return "Volume cannot be mounted"
	case "FailedCreate":
		return "Controller cannot create pods"
	case "FailedCreatePodSandBox", "NetworkNotReady":
		return "Pod networking cannot be set up"
	default:
		if summary, ok := kubeletEventSummaries[note.Reason]; ok {
			return summary
		}
		return "Kubernetes reported " + note.Reason
	}
}

// startReasons are events of a pod getting ready to run: its volumes,
// devices and network. The kubelet retries them, and a short failure while
// the token volume is prepared is common on a busy node.
var startReasons = map[string]bool{
	"FailedMount":                         true,
	"FailedAttachVolume":                  true,
	"FailedCreatePodSandBox":              true,
	"NetworkNotReady":                     true,
	reasons.FailedMapVolume:               true,
	reasons.FailedPrepareDynamicResources: true,
}

// overcomeAtStart reports whether note is a start-up failure of a pod that
// became ready after it: the retry worked, so the event is history, not a
// failure. A failure after the pod became ready still counts.
func overcomeAtStart(e inventory.Entity, note inventory.Note) bool {
	if e.ID.Kind != kube.KindPod || !startReasons[note.Reason] ||
		!flag(e, kube.AttrReady) {
		return false
	}
	readySince := timestamp(e, kube.AttrReadySince)
	return !readySince.IsZero() && !readySince.Before(note.At)
}

// deleted reports whether e was an object of a watched kind that is gone.
// Its events outlive it by up to EventWindow, but a deleted Pod no longer
// fails: keeping its findings would hold an incident open after the fix.
// Events about kinds kwatch does not watch have no object to check and
// still count.
func deleted(ctx detection.Context, e inventory.Entity) bool {
	return ctx.Synced(e.ID.Kind) && !ctx.Model.Exists(e.ID)
}
