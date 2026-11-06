package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A rollout that cannot start while the previous version still serves
// is not an outage. The pods of the new template crash or never get
// ready, but the old ReplicaSet's pods still answer: users are served.
// Such an incident notifies; it pages only while availability is
// actually lost (no ready replica, a route without backends), and an
// incident that paged during a real outage falls back to a
// notification once the previous version serves again, instead of
// staying "down" at the tier it reached (ratchetTier). Writers word it
// as such (Incident.PreviousServes).

// previousServes reports a Deployment root whose ready replicas cover
// its desired count while some of its pods fail, and whose ready pods
// belong to another ReplicaSet than the failing ones: a rollout whose
// new pods fail while the previous version's still serve.
func previousServes(model inventory.Reader, p *Incident) bool {
	if p.Root.Kind != kube.KindDeployment {
		return false
	}
	desired, ready, ok := ReplicasOf(model, p.Root)
	if !ok || desired == 0 || ready < desired {
		return false
	}
	failingSets := map[inventory.EntityID]bool{}
	var readySets []inventory.EntityID
	for _, pod := range rootcause.OwnedPods(model, p.Root) {
		for _, rs := range model.Related(pod, inventory.OwnedBy,
			inventory.Outgoing) {
			switch {
			case podReady(model, pod):
				readySets = append(readySets, rs)
			case podRestarts(model, pod) > 0:
				failingSets[rs] = true
			}
		}
	}
	if len(failingSets) == 0 {
		return false
	}
	for _, rs := range readySets {
		if !failingSets[rs] {
			return true
		}
	}
	return false
}

// easePage lowers a page to a notification when nothing pages any more
// (pageRuleOf) and the previous version serves: the outage is over and
// only the rollout fails. Overrides still apply. Nothing is sent: the
// tier change is in the journal, and a new outage pages again through
// the usual raise.
func (m *Manager) easePage(p *Incident, now time.Time) {
	if p.Tier != Page || !p.PreviousServes || pageRuleOf(p) != "" ||
		m.inGrace(p, now) || !wasAnnounced(p) {
		return
	}
	base, why := tierWhy(p)
	computed := m.override.apply(p, base)
	if computed >= Page {
		return
	}
	p.Tier = computed
	// The pagers opened a page: they are owed the lowering, once.
	p.Pending.lowered = p.Delivery.SentTier() >= Page
	p.noteLevel(m.levelFor(p, base, why, computed))
	p.note(now, "tier lowered: the previous version serves again; only "+
		"the new rollout fails")
	logLifecycle(p, "tier", "incident tier changed", "from", Page.String())
}

// loweredStep sends the update owed after easePage lowered a page: one
// "tier lowered" update, which the delivery routes to the providers that
// keep alerts by key only (PagerDuty takes the new severity, Opsgenie the
// new priority). It is not held by MaterialGap: an on-call person paged
// for an outage should learn soon that users are served again. Sending
// it records the lower tier as sent, so it goes out once per lowering.
// When anything else changed too, it is a material update to everyone.
func (m *Manager) loweredStep(p *Incident, now time.Time) (Decision, bool) {
	if !p.Pending.lowered {
		return Decision{}, false
	}
	p.Pending.lowered = false
	sent := p.Delivery.SentTier()
	if p.Tier >= Page || sent < Page {
		// Paging again, or the lower tier was told meanwhile.
		return Decision{}, false
	}
	if onlyTierChanged(p, sent) {
		return m.decide(p, Update, ReasonTierLowered), true
	}
	// Other news came with it: one material update tells everyone,
	// pagers included (they take its lower severity), not held by
	// MaterialGap.
	p.Delivery.MarkMaterial(now)
	return m.decide(p, Update, ReasonMaterialChange), true
}

// onlyTierChanged reports that p's fingerprint differs from the one
// last told only by its tier: with the tier sent, it would match.
func onlyTierChanged(p *Incident, sent Tier) bool {
	tier := p.Tier
	p.Tier = sent
	same := fingerprint(p) == p.Digest
	p.Tier = tier
	return same
}
