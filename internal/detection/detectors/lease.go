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
//
// leaseGapMin is the least gap between two observed renewals that still
// counts as a rhythm: scans are far apart, so a Lease of a few seconds is
// seen to renew one scan later at best, and the allowance has room for one
// scan missed.
const (
	leaseStaleFactor = 3
	leaseStaleMargin = time.Minute
	leaseStaleMin    = 2*kube.LeaseScanPeriod + leaseStaleMargin
	leaseGapMin      = 2 * kube.LeaseScanPeriod
)

// Lease detects a controller or operator that holds its leader Lease but
// stopped renewing it, and nobody took over. A holder that runs and is
// ready is alive and does nothing, which no pod status shows; a holder
// that crash loops or is stuck says why, and its own incident explains
// the Lease. A Lease whose holder is gone is left alone; it is what an
// uninstalled controller leaves behind.
//
// Some programs take a Lease once at startup and renew it only when they
// restart, so a healthy holder counts only once kwatch saw it renew in a
// rhythm (see leaseRenews): an old renew time alone says nothing about
// that holder. A restart of kwatch forgets the rhythm, and a healthy
// holder is then left alone until it is seen to renew again.
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
	seconds, _ := number(e, kube.AttrLeaseDuration)
	stale := max(time.Duration(seconds*leaseStaleFactor)*time.Second,
		leaseStaleMin)
	renews := leaseRenews(ctx, holder, renewed, seconds)
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

// leaseRenews reports whether kwatch saw holder renew its Lease in a
// rhythm: two renew times in a row, read by two evaluations, no further
// apart than three durations (at least leaseGapMin). One slow bump, such
// as a program taking the Lease again when it restarts, sets nothing.
//
// The rhythm is kept for as long as the holder stays, in memory per holder.
// A late renewal after a long stall does not undo it: the holder renewed
// before and stalled, which is what is reported. A new holder starts over,
// and the state of the one before is forgotten with it.
func leaseRenews(
	ctx detection.Context, holder string, renewed time.Time, seconds float64,
) bool {
	allowed := max(time.Duration(seconds*leaseStaleFactor)*time.Second,
		leaseGapMin)
	before, seen := ctx.Track("lease-renewed/"+holder, renewed)
	gap := renewed.Sub(before)
	const proven = "lease-rhythm/"
	if ctx.Ongoing(proven+holder) || (seen && gap > 0 && gap <= allowed) {
		ctx.Onset(proven+holder, renewed)
		return true
	}
	return false
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
