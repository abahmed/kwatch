package kube

import (
	"context"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// leases records every Lease outside the node heartbeats and the
// control-plane leaders: who holds it, when it was last renewed and
// for how long it is valid. The detector decides what is stale; the
// prober only reports. An error (no RBAC, a managed control plane that
// hides them) reports nothing. A Lease the last complete scan reported
// and this one did not is retired (retireLease). An incomplete scan (a
// failed page, too many pages) retires nothing.
func (p *Prober) leases(ctx context.Context) []inventory.Observation {
	items, complete := p.listLeases(ctx)
	now := p.cfg.Now()
	var out []inventory.Observation
	seen := make(map[inventory.EntityID]bool, len(items))
	listed := make(map[inventory.EntityID]bool, len(items))
	for _, lease := range items {
		id := inventory.CoreID(KindLease, lease.Namespace, lease.Name)
		listed[id] = true
		if p.skipLease(lease) {
			continue
		}
		seen[id] = true
		out = append(out, p.leaseObservations(id, lease, now)...)
	}
	if !complete {
		for id := range p.leasesSeen {
			seen[id] = true
		}
	}
	for id := range p.leasesSeen {
		if !seen[id] {
			out = append(out, p.retireLease(id, listed[id], now)...)
		}
	}
	p.leasesSeen = seen
	return out
}

// leaseObservations are what the scan says about one reported Lease: its
// holder and renewal, and the edge to the holder's pod.
func (p *Prober) leaseObservations(
	id inventory.EntityID, lease coordinationv1.Lease, now time.Time,
) []inventory.Observation {
	seconds := float64(defaultLeaseSeconds)
	if lease.Spec.LeaseDurationSeconds != nil {
		seconds = float64(*lease.Spec.LeaseDurationSeconds)
	}
	holder := *lease.Spec.HolderIdentity
	return []inventory.Observation{{
		Kind: inventory.Observed, Source: ProbeSource, At: now,
		Entity: id,
		Attributes: map[string]inventory.Value{
			AttrLeaseHolder:   inventory.Text(holder),
			AttrLeaseRenewed:  inventory.Time(lease.Spec.RenewTime.Time),
			AttrLeaseDuration: inventory.Number(seconds),
		},
	}, p.holderRelation(id, holder, now)}
}

// retireLease is what the scan says about a Lease it reported before and
// not now. One missing from the list is gone. One still listed but now
// left out (released, or no longer renewed) still exists: when a watch
// lists Leases too, that watch owns the object, so the prober withdraws
// only its own attributes and holder edge, and the Lease, its other
// relations and its history stay. Without such a watch the prober is the
// Lease's only source, and the Lease leaves the model.
func (p *Prober) retireLease(
	id inventory.EntityID, listed bool, now time.Time,
) []inventory.Observation {
	if !listed || p.cfg.LeasesWatched == nil || !p.cfg.LeasesWatched() {
		return []inventory.Observation{{
			Kind: inventory.Gone, Source: ProbeSource, At: now, Entity: id,
		}}
	}
	return []inventory.Observation{{
		Kind: inventory.Observed, Source: ProbeSource, At: now, Entity: id,
		Attributes: map[string]inventory.Value{},
	}, {
		Kind: inventory.Related, Source: ProbeSource, At: now, Entity: id,
		Relation: inventory.References,
	}}
}

// skipLease is a Lease the scan leaves out: node heartbeats, the
// control-plane leaders, kwatch's own and one nobody holds or renews.
func (p *Prober) skipLease(lease coordinationv1.Lease) bool {
	return lease.Namespace == nodeLeaseNamespace ||
		lease.Spec.RenewTime == nil || lease.Spec.HolderIdentity == nil ||
		p.isOwnLease(lease.Namespace, lease.Name) ||
		(lease.Namespace == "kube-system" &&
			(lease.Name == Scheduler.Name ||
				lease.Name == ControllerManager.Name))
}

// isOwnLease reports kwatch's own leader Lease.
func (p *Prober) isOwnLease(namespace, name string) bool {
	return p.cfg.OwnLeaseName != "" && name == p.cfg.OwnLeaseName &&
		namespace == p.cfg.OwnLeaseNamespace
}
