package detectors

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// loadBalancerFailures are the cloud service controller's Warning events
// about a Service's load balancer, with the severity of each: without a
// synced load balancer the Service has no external address at all.
var loadBalancerFailures = map[string]detection.Severity{
	"SyncLoadBalancerFailed":   detection.Critical,
	"UpdateLoadBalancerFailed": detection.Warning,
	"DeleteLoadBalancerFailed": detection.Warning,
	"UnAvailableLoadBalancer":  detection.Warning,
}

// loadBalancerEvents reports recent load balancer controller failures as
// one finding carrying the controller's messages, which name the cloud
// error (quota, permissions, subnet, annotation).
func loadBalancerEvents(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if ctx.Model == nil {
		return nil
	}
	latest := latestLoadBalancerNotes(ctx, e.ID)
	dropSettledNotes(latest, e)
	if len(latest) == 0 {
		return nil
	}
	names := make([]string, 0, len(latest))
	for reason := range latest {
		names = append(names, reason)
	}
	sort.Strings(names)
	f := detection.Finding{
		Reason: reasons.LoadBalancerSyncFailed, Severity: detection.Warning,
		Summary: "Cloud load balancer controller failed (" +
			strings.Join(names, ", ") + ")",
	}
	for _, reason := range names {
		note := latest[reason]
		ctx.RecheckAfter(note.At.Add(EventWindow).Sub(ctx.Now))
		if loadBalancerFailures[reason] > f.Severity {
			f.Severity = loadBalancerFailures[reason]
		}
		if f.Since.IsZero() || note.At.Before(f.Since) {
			f.Since = note.At
		}
		f.Evidence = append(f.Evidence,
			detection.Evidence{Label: reason, Value: note.Message})
	}
	return []detection.Finding{f}
}

// dropSettledNotes forgets failures older than the Service's address: a
// balancer that has an address works now, so only a newer failure counts.
func dropSettledNotes(latest map[string]inventory.Note, e inventory.Entity) {
	if !flag(e, kube.AttrLoadBalancer) {
		return
	}
	addressed := valueSince(e, kube.AttrLoadBalancer)
	for reason, note := range latest {
		if !note.At.After(addressed) {
			delete(latest, reason)
		}
	}
}

// latestLoadBalancerNotes keeps the newest recent note of each load
// balancer failure reason.
func latestLoadBalancerNotes(
	ctx detection.Context, id inventory.EntityID,
) map[string]inventory.Note {
	latest := map[string]inventory.Note{}
	for _, note := range ctx.Model.Notes(id, ctx.Now.Add(-EventWindow)) {
		if _, ok := loadBalancerFailures[note.Reason]; !ok || !note.Warning {
			continue
		}
		if seen, ok := latest[note.Reason]; !ok || note.At.After(seen.At) {
			latest[note.Reason] = note
		}
	}
	return latest
}
