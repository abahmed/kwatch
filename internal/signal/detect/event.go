package detect

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/signal"
)

// EventWindow is how recent a Warning event must be to count as current.
const EventWindow = 15 * time.Minute

// eventReasons are Warning event reasons that are failures on their own
// and are not already detected from object state. Reasons that state
// already shows (BackOff, Unhealthy, FailedScheduling, Evicted, HPA and
// Job conditions) stay evidence only, so nothing is reported twice.
var eventReasons = map[string]signal.Severity{
	"FailedMount":            signal.Critical,
	"FailedAttachVolume":     signal.Critical,
	"FailedCreatePodSandBox": signal.Critical,
	"FailedCreate":           signal.Critical,
	"FailedCallingWebhook":   signal.Critical,
	"ProvisioningFailed":     signal.Warning,
	"FailedBinding":          signal.Warning,
	"VolumeResizeFailed":     signal.Warning,
	"FailedDaemonPod":        signal.Warning,
	"NetworkNotReady":        signal.Critical,
	"FailedKillPod":          signal.Warning,
	"FailedToScaleUp":        signal.Warning,
	"NotTriggerScaleUp":      signal.Warning,
	"FailedPreStopHook":      signal.Warning,
	"FailedPostStartHook":    signal.Critical,
}

// Event turns recent failure-shaped Warning events into signals for any
// entity kind.
type Event struct{}

// Name implements signal.Detector.
func (Event) Name() string { return "event" }

// Kinds implements signal.Detector.
func (Event) Kinds() []knowledge.Kind {
	return []knowledge.Kind{signal.AnyKind}
}

// Detect implements signal.Detector.
func (Event) Detect(ctx signal.Context, e knowledge.Entity) []signal.Signal {
	var out []signal.Signal
	for _, note := range ctx.Model.Notes(e.ID, ctx.Now.Add(-EventWindow)) {
		severity, failing := eventReasons[note.Reason]
		if !note.Warning || !failing {
			continue
		}
		ctx.RecheckAfter(note.At.Add(EventWindow).Sub(ctx.Now))
		evidence := []signal.Evidence{{Label: "event", Value: note.Message}}
		if note.Count > 1 {
			evidence = append(evidence, signal.Evidence{
				Label: "occurrences", Value: strconv.Itoa(note.Count),
			})
		}
		out = append(out, signal.Signal{
			Reason: note.Reason, Severity: severity, Since: note.At,
			Summary:  eventSummary(note),
			Evidence: evidence,
		})
	}
	return out
}

func eventSummary(note knowledge.Note) string {
	switch note.Reason {
	case "FailedMount", "FailedAttachVolume":
		return "Volume cannot be mounted"
	case "FailedCreate":
		return "Controller cannot create pods"
	case "FailedCallingWebhook":
		return "An admission webhook is failing requests"
	case "FailedCreatePodSandBox", "NetworkNotReady":
		return "Pod networking cannot be set up"
	case "FailedToScaleUp", "NotTriggerScaleUp":
		return "Cluster autoscaler cannot add nodes"
	default:
		return "Kubernetes reported " + note.Reason
	}
}
