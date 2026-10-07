package detectors

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultLoadBalancerPending is how long a LoadBalancer may wait for an
// address; cloud load balancers normally provision within minutes.
const DefaultLoadBalancerPending = 5 * time.Minute

// DefaultLoadBalancerFailing is the wait once the controller has
// reported a failure for the Service: it retries on its own, and a
// minute or two of retries is normal.
const DefaultLoadBalancerFailing = 2 * time.Minute

// eventKeepTime is how long Kubernetes keeps an Event. A Service that
// has waited longer may have had its events expire, so silence from
// the controllers proves nothing then.
const eventKeepTime = time.Hour

// loadBalancerWaiting reports a LoadBalancer Service that has no address
// after its type was set. The Service is the root, so the controller's
// events about it (a FailedDeployModel, a SyncLoadBalancerFailed) are
// quoted here instead of becoming findings of their own. Only a Service
// that others depend on, or that had an address and lost it, is worth an
// interruption; a brand-new one waits for the digest.
func loadBalancerWaiting(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if !loadBalancerUnaddressed(e) {
		return nil
	}
	since := loadBalancerWaitSince(e)
	note, acted := latestServiceNote(ctx, e.ID, since)
	// A controller that already reported a failure needs no long wait
	// to be believed: it said it cannot build the load balancer. The
	// cloud provider's own sync failures are reported by
	// loadBalancerEvents at once, so they do not shorten this one.
	wait := DefaultLoadBalancerPending
	_, syncFailure := loadBalancerFailures[note.Reason]
	if acted && note.Warning && !syncFailure {
		wait = DefaultLoadBalancerFailing
	}
	if !sustained(ctx, "lb-pending", since, wait) {
		return nil
	}
	f := detection.Finding{
		Reason: reasons.LoadBalancerPending, Severity: detection.Info,
		Since: since,
	}
	if hadAddress(e) || ctx.Model != nil && len(ctx.Model.Related(
		e.ID, inventory.RoutesTo, inventory.Incoming)) > 0 {
		f.Severity = detection.Warning
	}
	waited := "has been waiting " + minutesText(ctx.Now.Sub(since)) +
		" for a load balancer"
	switch {
	case acted:
		f.Summary = waited + ": \"" + note.Message + "\""
		f.Evidence = []detection.Evidence{
			{Label: note.Reason, Value: note.Message}}
	case ctx.Now.Sub(since) <= eventKeepTime:
		f.Summary = waited + ": " + noControllerText(e)
	default:
		f.Summary = waited
	}
	return []detection.Finding{f}
}

// loadBalancerUnaddressed reports a LoadBalancer Service with no address.
func loadBalancerUnaddressed(e inventory.Entity) bool {
	return text(e, kube.AttrServiceType) == "LoadBalancer" &&
		!flag(e, kube.AttrLoadBalancer)
}

// loadBalancerWaitSince is when the wait began: the later of the Service
// losing its address and its type becoming LoadBalancer.
func loadBalancerWaitSince(e inventory.Entity) time.Time {
	since := valueSince(e, kube.AttrLoadBalancer)
	if typed := valueSince(e, kube.AttrServiceType); typed.After(since) {
		return typed
	}
	return since
}

// hadAddress reports a Service whose address came and went: its
// addressed state flipped at least once.
func hadAddress(e inventory.Entity) bool {
	attribute, ok := e.Attribute(kube.AttrLoadBalancer)
	return ok && len(attribute.Flips) > 0
}

// latestServiceNote is the newest event any controller recorded about
// the Service since the wait began, a Warning in preference to a Normal
// one: "Ensuring load balancer" says less than the failure after it.
func latestServiceNote(
	ctx detection.Context, id inventory.EntityID, since time.Time,
) (inventory.Note, bool) {
	var best inventory.Note
	found := false
	if ctx.Model == nil {
		return best, false
	}
	for _, note := range ctx.Model.Notes(id, since) {
		better := !found || note.Warning && !best.Warning ||
			note.Warning == best.Warning && note.At.After(best.At)
		if better {
			best, found = note, true
		}
	}
	return best, found
}

// noControllerText says nothing has reacted to the Service.
func noControllerText(e inventory.Entity) string {
	if class := text(e, kube.AttrLoadBalancerClass); class != "" {
		return "no controller for load balancer class " + class +
			" has acted on it"
	}
	return "no controller has acted on it"
}

// minutesText is a wait in whole minutes: "40 min".
func minutesText(d time.Duration) string {
	return strconv.Itoa(int(d.Minutes())) + " min"
}

// quotedByService reports an AWS-style load balancer controller event
// that the Service's own waiting finding quotes already, so it is not
// also an unusual event.
func quotedByService(e inventory.Entity, reason string) bool {
	return (reason == "FailedDeployModel" || reason == "FailedBuildModel") &&
		e.ID.Kind == kube.KindService && loadBalancerUnaddressed(e)
}
