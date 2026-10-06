package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// LinkSlows is a cause that makes the effect slow without refusing
// anything, as an admission webhook that takes seconds per call slows
// the API server's writes.
const LinkSlows LinkType = "slows"

// slowHookModes are the modes of an admission webhook whose calls slow
// the API server down: it is slow, or it fails and is retried.
var slowHookModes = []detection.Mode{
	detection.ModeWebhookSlow, detection.ModeWebhookRejecting,
}

// apiLatencyRows blame an admission webhook for a slow API server. The
// API server's own metrics say its writes are slow and the webhook's say
// its calls are; the webhook is the cause only when it is slow too.
var apiLatencyRows = []Row{{
	Name:  "webhook-slows-api",
	Cause: Side{Group: AnyGroup, Kind: AnyKind, Modes: slowHookModes},
	Link:  LinkSlows,
	Effect: Side{Kind: kindAPIServer, Modes: []detection.Mode{
		detection.ModeLatencyWrites}},
	Prior: 0.75,
}}

// slowWebhookHops lead from a slow API server to the admission
// webhooks that are slow or failing themselves.
func (v *view) slowWebhookHops() []hop {
	var out []hop
	for _, kind := range admitKinds {
		for _, hook := range v.s.Model.Entities(kind) {
			if v.hasMode(hook, slowHookModes) {
				out = append(out, hop{link: LinkSlows, to: hook})
			}
		}
	}
	return out
}

// apiLatencyHops are the hops out of an API server whose writes are
// slow. Only that mode leads to webhooks: a healthy one fans out to
// nothing.
func (v *view) apiLatencyHops(id inventory.EntityID) []hop {
	if id.Kind != kindAPIServer ||
		!v.hasMode(id, []detection.Mode{detection.ModeLatencyWrites}) {
		return nil
	}
	return v.slowWebhookHops()
}
