package detectors

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// backend is one Service a webhook configuration calls, as the webhook
// detector judges it.
type backend struct {
	config, service inventory.EntityID
	// blocking is set when a webhook failing closed calls it, and
	// rejected once a request was refused because of it.
	blocking, rejected bool
	// reach is what the fail-closed webhooks' rules intercept among the
	// objects that exist now; only read when blocking.
	reach kube.WebhookReach
}

// maxReachNames bounds the resources a summary names.
const maxReachNames = 3

// finding is the finding of a dead backend: summary says what is wrong
// with it and gone how the reach sentence says it.
func (b backend) finding(
	reason, summary, gone string,
) detection.Finding {
	f := detection.Finding{Reason: reason, Severity: b.severity(),
		Summary: summary + b.effect()}
	if b.rejecting() {
		names := reachNames(b.reach.Resources)
		f.Summary = "Admission webhook rejects creates and updates of " +
			names + " right now: it fails closed and its backend " +
			"Service " + b.service.Name + " " + gone
		f.Evidence = []detection.Evidence{{
			Label: detection.EvidenceWebhookIntercepts,
			Value: strings.Join(b.reach.Resources, ", ")}}
	}
	return f
}

// rejecting reports a fail-closed backend whose rules intercept objects
// that exist now: their creates and updates are refused, whether or
// not one was tried yet.
func (b backend) rejecting() bool {
	return b.blocking && !b.rejected && len(b.reach.Resources) > 0
}

// severity is how much a dead webhook backend matters. With
// failurePolicy Ignore, requests skip it and nothing is blocked: that is
// only worth the digest. With Fail it blocks every request the webhook
// matches, but it is critical only once a request was refused: an
// operator's leftover webhook matches only that operator's own objects
// and may never be called again. Until then it is a warning, and only
// worth the digest when its rules are known to match no object that
// exists now (resources whose definition is gone).
func (b backend) severity() detection.Severity {
	switch {
	case b.rejected:
		return detection.Critical
	case b.blocking && !b.reach.None():
		return detection.Warning
	}
	return detection.Info
}

// effect says what a dead webhook backend does to requests.
func (b backend) effect() string {
	switch {
	case b.rejected:
		return "; it blocks matching requests (failurePolicy Fail)"
	case b.blocking && b.reach.None():
		return "; its rules match no object that exists now " +
			"(failurePolicy Fail)"
	case b.blocking && b.reach.Listed && b.reach.Unknown:
		return "; no request has been rejected yet (failurePolicy " +
			"Fail), and kwatch cannot tell whether its rules match " +
			"objects that exist now"
	case b.blocking:
		return "; no request has been rejected yet (failurePolicy Fail)"
	}
	return "; requests skip it because failurePolicy is Ignore"
}

// reachNames names at most maxReachNames resources: "pods",
// "pods and services", "configmaps, pods, services and 2 more".
func reachNames(resources []string) string {
	if len(resources) > maxReachNames {
		more := len(resources) - maxReachNames
		return strings.Join(resources[:maxReachNames], ", ") + " and " +
			strconv.Itoa(more) + " more"
	}
	if len(resources) <= 1 {
		return strings.Join(resources, "")
	}
	last := len(resources) - 1
	return strings.Join(resources[:last], ", ") + " and " + resources[last]
}
