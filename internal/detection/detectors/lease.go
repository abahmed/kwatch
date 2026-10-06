package detectors

import (
	"strings"
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
// stopped renewing it while its pod runs and is ready: the process is alive and
// does nothing, which no pod status shows. A Lease whose holder is gone
// is left alone; it is what an uninstalled controller leaves behind.
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
	age := ctx.Now.Sub(renewed)
	if age < stale {
		ctx.RecheckAfter(stale - age)
		return nil
	}
	pod, ok := holderPod(ctx.Model, e.ID.Namespace, holder)
	if !ok {
		return nil
	}
	return []detection.Finding{{
		Reason: reasons.LeaseStale, Severity: detection.Warning,
		Since: renewed.Add(stale),
		Summary: "Lease held by pod " + pod.Name + " has not been renewed " +
			"for " + format.Duration(age) + "; the controller runs but " +
			"does not work",
		Evidence: []detection.Evidence{{Label: "holder", Value: holder}},
	}}
}

// holderPod finds the running, ready pod a holder identity names. Leader
// election writes "<pod>_<id>"; the pod name is the part before the
// underscore, or the whole identity when there is none.
func holderPod(
	model inventory.Reader, namespace, holder string,
) (inventory.EntityID, bool) {
	name, _, _ := strings.Cut(holder, "_")
	id := inventory.CoreID(kube.KindPod, namespace, name)
	pod, ok := model.Entity(id)
	// A pod that is not Running and Ready (crash looping, restarting,
	// starting) explains the stale lease with its own findings.
	if !ok || text(pod, kube.AttrPhase) != "Running" ||
		!flag(pod, kube.AttrReady) {
		return inventory.EntityID{}, false
	}
	return id, true
}
