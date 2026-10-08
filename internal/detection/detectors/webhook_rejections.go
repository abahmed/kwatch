package detectors

import (
	"regexp"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// webhookCreators are the kinds whose controllers record FailedCreate
// when the API server refuses the pods they ask for.
var webhookCreators = []inventory.Kind{
	kube.KindReplicaSet, kube.KindStatefulSet, kube.KindDaemonSet,
	kube.KindJob,
}

// failedWebhookCall finds the webhook the API server failed to call, in
// its own words: failed calling webhook "x.example.com": ...
var failedWebhookCall = regexp.MustCompile(`failed calling webhook "([^"]+)"`)

// IsWebhookRejection reports whether an event says the API server
// refused a request because it could not call a webhook. The event lands
// on the controller that made the request, yet it is about the webhook:
// the engine judges the webhook again when one arrives.
func IsWebhookRejection(note inventory.Note) bool {
	return note.Warning && failedWebhookCall.MatchString(note.Message)
}

// webhookRejected reports whether a request was refused lately because
// of this webhook configuration: the API server's own metrics count
// calls that failed and so rejected the request, or a controller was
// told that a webhook of the configuration could not be called. A
// webhook that fails open rejects nothing, so its failed calls do not
// count. A dead backend that nothing has called yet has rejected
// nothing: it will reject the first create it matches.
func webhookRejected(ctx detection.Context, e inventory.Entity) bool {
	if share, ok := number(e, kube.AttrWebhookClosedShare); ok &&
		share > 0 {
		return true
	}
	if ctx.Model == nil {
		return false
	}
	closed := failClosedHooks(e)
	if len(closed) == 0 {
		return false
	}
	since := ctx.Now.Add(-EventWindow)
	for _, kind := range webhookCreators {
		for _, id := range ctx.Model.Entities(kind) {
			for _, note := range ctx.Model.Notes(id, since) {
				if !refusedBy(note, closed) {
					continue
				}
				ctx.RecheckAfter(note.At.Add(EventWindow).
					Sub(ctx.Now) + time.Nanosecond)
				return true
			}
		}
	}
	return false
}

// failClosedHooks are the names of the configuration's webhooks that
// refuse a request when they cannot be called. A missing policy reads as
// Fail, the API server's default.
func failClosedHooks(e inventory.Entity) []string {
	names := strings.Split(text(e, kube.AttrWebhookNames), ",")
	policies := strings.Split(text(e, kube.AttrFailurePolicy), ",")
	var out []string
	for i, name := range names {
		if name == "" || (i < len(policies) &&
			strings.EqualFold(policies[i], "Ignore")) {
			continue
		}
		out = append(out, strings.ToLower(name))
	}
	return out
}

// refusedBy reports whether the note says a call to one of the webhooks
// failed.
func refusedBy(note inventory.Note, hooks []string) bool {
	if !note.Warning {
		return false
	}
	message := strings.ToLower(note.Message)
	for _, m := range failedWebhookCall.FindAllStringSubmatch(message, -1) {
		for _, hook := range hooks {
			if m[1] == hook {
				return true
			}
		}
	}
	return false
}
