package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// leaseStaleFactor is how many lease durations may pass without a
// renewal before the holder counts as stuck; leaseStaleMin keeps very
// short leases from flagging a slow API server.
//
// leaseStaleMin must stay well above the time between two scans of the
// Lease objects: the model only learns of a renewal at the next scan, so
// a younger limit would flag a healthy holder whose latest renewal is
// simply not read yet. It is two scans plus a margin, taken from the scan
// period itself so the two cannot drift apart.
const (
	leaseStaleFactor = 3
	leaseStaleMargin = time.Minute
	leaseStaleMin    = 2*kube.LeaseScanPeriod + leaseStaleMargin
)

// Lease detects a controller or operator that holds its leader Lease but
// stopped renewing it, and nobody took over. A holder that runs and is
// ready is alive and does nothing, which no pod status shows; a holder
// that crash loops or is stuck says why, and its own incident explains
// the Lease. A Lease whose holder is gone is left alone; it is what an
// uninstalled controller leaves behind.
//
// Some programs take a Lease once at startup and renew it only when they
// restart, so a healthy holder counts only once it is known to renew: an
// old renew time alone says nothing about that holder. The Lease itself
// tells (see leaseRenews), which survives a restart of kwatch and is not
// fooled by a holder of a stable identity taking the Lease again.
type Lease struct{}

// Name implements detection.Detector.
func (Lease) Name() string { return "lease" }

// Kinds implements detection.Detector.
func (Lease) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindLease}
}

// Detect implements detection.Detector.
func (Lease) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	renewed := timestamp(e, kube.AttrLeaseRenewed)
	holder := text(e, kube.AttrLeaseHolder)
	if renewed.IsZero() || holder == "" {
		return nil
	}
	renews := leaseRenews(ctx, e, holder, renewed)
	seconds, _ := number(e, kube.AttrLeaseDuration)
	stale := max(time.Duration(seconds*leaseStaleFactor)*time.Second,
		leaseStaleMin)
	age := ctx.Now.Sub(renewed)
	if age < stale {
		ctx.RecheckAfter(stale - age)
		return nil
	}
	pod, state, ok := holderPod(ctx.Model, e.ID.Namespace, holder)
	if !ok || (state == "" && !renews) {
		return nil
	}
	summary := "Lease held by pod " + pod.Name + " has not been renewed " +
		"for " + format.Duration(age) + "; the controller runs but " +
		"does not work"
	if state != "" {
		summary = "Lease held by pod " + pod.Name + " has not been " +
			"renewed for " + format.Duration(age) + "; the pod is " +
			state + ": the controller is not acting"
	}
	return []detection.Finding{{
		Reason: reasons.LeaseStale, Severity: detection.Warning,
		Since: renewed.Add(stale), Summary: summary,
		Evidence: []detection.Evidence{{Label: "holder", Value: holder}},
	}}
}

// leaseRenewMargin is how far apart acquire and renew times must be to
// be two writes: a program may stamp them separately in one acquire.
const leaseRenewMargin = time.Second

// leaseRenews reports whether the holder renews its Lease. Taking a Lease
// sets the acquire time, also for an identity taking it again after a
// restart, so a renew time past it is a renewal. A Lease without an
// acquire time falls back on what this process saw.
func leaseRenews(
	ctx detection.Context, e inventory.Entity, holder string,
	renewed time.Time,
) bool {
	acquired := timestamp(e, kube.AttrLeaseAcquired)
	if acquired.IsZero() {
		return renewalSeen(ctx, holder, renewed)
	}
	return renewed.Sub(acquired) > leaseRenewMargin
}

// renewalSeen reports whether holder renewed the Lease since kwatch first
// saw it hold it: the first renew time is kept for as long as the holder
// stays, and a later one proves it renews.
func renewalSeen(
	ctx detection.Context, holder string, renewed time.Time,
) bool {
	return renewed.After(ctx.Onset("lease-renewal/"+holder, renewed))
}

// holderPod finds the pod a holder identity names. state is what is
// wrong with it, empty for a pod that runs and is ready. A holder that
// is gone or finished is left alone: an uninstalled controller leaves
// its Lease behind, and a Job's pod ends with Succeeded.
func holderPod(
	model inventory.Reader, namespace, holder string,
) (inventory.EntityID, string, bool) {
	id := inventory.CoreID(kube.KindPod, namespace,
		kube.HolderPodName(holder))
	pod, ok := model.Entity(id)
	if !ok || text(pod, kube.AttrPhase) == "Succeeded" {
		return inventory.EntityID{}, "", false
	}
	if state := podTrouble(model, pod); state != "" {
		return id, state, true
	}
	if text(pod, kube.AttrPhase) != "Running" || !flag(pod, kube.AttrReady) {
		return id, "not ready", true
	}
	return id, "", true
}
