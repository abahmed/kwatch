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
	for _, note := range ctx.Model.Notes(e.ID, ctx.Now.Add(-maxNoteLife)) {
		severity, failing := detection.EventSeverity(note.Reason)
		deadline := noteDeadline(e, note)
		if ctx.Now.After(deadline) || !note.Warning || !failing ||
			overcomeAtStart(e, note) ||
			recoveredBefore(e, note) || bootNoise(ctx, e, note) ||
			daemonNodeNoise(ctx, e, note) || staleNodeNote(e, note) {
			continue
		}
		// A note exactly as old as its life still counts, so the
		// recheck falls just after it: the finding must clear when
		// its last event ages out.
		ctx.RecheckAfter(deadline.Sub(ctx.Now) + time.Nanosecond)
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
		if key == reasons.VolumeMultiAttach && !sustained(ctx,
			"multi-attach", ctx.Onset("multi-attach", byReason[key].first),
			DefaultMultiAttachHold) {
			continue
		}
		found := byReason[key].finding(key)
		if key == reasons.VolumeMultiAttach {
			found.Evidence = append(found.Evidence, heldEvidence(ctx, e,
				byReason[key].newest.Message)...)
		}
		out = append(out, found)
	}
	return append(out, unusualEvents(ctx, e)...)
}

// eventGroup folds the notes of one reason: notes are kept per source,
// but findings are keyed by reason.
type eventGroup struct {
	newest   inventory.Note
	first    time.Time
	severity detection.Severity
	count    int
}

func (g *eventGroup) add(note inventory.Note, severity detection.Severity) {
	if g.first.IsZero() || note.At.Before(g.first) {
		g.first = note.At
	}
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
// ran after it: the retry worked, so the event is history, not a failure.
// A pod that became ready after the note, or whose containers started
// after it, overcame it even if it is not ready now (its node may be
// lost). A failure after that still counts.
func overcomeAtStart(e inventory.Entity, note inventory.Note) bool {
	if e.ID.Kind != kube.KindPod || !startReasons[note.Reason] {
		return false
	}
	started := timestamp(e, kube.AttrContainersStarted)
	if !started.IsZero() && !started.Before(note.At) {
		return true
	}
	if !flag(e, kube.AttrReady) {
		return false
	}
	readySince := timestamp(e, kube.AttrReadySince)
	return !readySince.IsZero() && !readySince.Before(note.At)
}

// bootReasons are the start-up events a node that has just joined
// produces while its network plugin comes up: the pod retries and
// the failure passes by itself.
var bootReasons = map[string]bool{
	"FailedCreatePodSandBox": true,
	"NetworkNotReady":        true,
}

// bootNoise reports a bootReasons event of a pod that starts in a
// booting node pool. Once the boot is over the event counts again, so a
// pod that still cannot start its sandbox is reported.
func bootNoise(
	ctx detection.Context, e inventory.Entity, note inventory.Note,
) bool {
	return e.ID.Kind == kube.KindPod && bootReasons[note.Reason] &&
		(bootGraceFor(ctx, e) > 0 || youngNodeNoise(ctx, e, note))
}

// deleted reports whether e was an object of a watched kind that is gone.
// Its events outlive it by up to EventWindow, but a deleted Pod no longer
// fails: keeping its findings would hold an incident open after the fix.
// Events about kinds kwatch does not watch have no object to check and
// still count.
func deleted(ctx detection.Context, e inventory.Entity) bool {
	return ctx.Synced(e.ID.Kind) && !ctx.Model.Exists(e.ID)
}
