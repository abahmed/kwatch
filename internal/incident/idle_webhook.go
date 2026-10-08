package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// An idle webhook is a fail-closed admission webhook whose backend is
// gone while no request was refused because of it: the detector keeps it
// at warning until one is. Such an incident is announced once, so that
// nobody is surprised by the first refused create, then waits for the
// digest. It does not page, and it is not said again ("still open").
// It becomes an ordinary incident the moment a request is refused: the
// finding turns critical, or the incident is rooted at a webhook that
// blocks creates (admissionBlocked), and the usual page rules apply.

// webhookBackendReason reports a finding about a webhook whose backend
// is missing or has nothing ready.
func webhookBackendReason(s detection.Finding) bool {
	return s.Reason == reasons.WebhookNoEndpoints ||
		s.Reason == reasons.WebhookBackendNotFound
}

// idleWebhook reports an incident of webhook backend findings alone,
// none of them critical, that is not blocking creates.
func idleWebhook(p *Incident) bool {
	idle := false
	for _, s := range p.Members {
		switch {
		case s.Advisory:
		case webhookBackendReason(s):
			idle = true
		case s.Severity > detection.Info:
			return false
		}
		if s.Severity == detection.Critical {
			return false
		}
	}
	return idle && !p.admissionBlocked
}

// noteRejection remembers that a request was refused because of the
// incident's webhooks, and when. The fact lasts as long as the incident
// does, like the page it justifies, and is saved with it (RejectionSeenAt):
// a restart does not make a page that was blocking creates prove itself
// again. Only a page restored from a record without the field, or one that
// never saw a refusal, is judged against its current findings.
func (p *Incident) noteRejection(now time.Time) {
	seen := p.admissionBlocked
	for _, s := range p.Members {
		if webhookBackendReason(s) && s.Severity == detection.Critical {
			seen = true
		}
	}
	if seen {
		p.rejectionSeen = true
		p.rejectionAt = now
	}
}

// unpaged reports a page that has nothing to stand on: its webhook
// findings are idle and no refusal was seen since this process started.
// That is a page restored from before the webhook rule asked for a
// refused request. It follows the members down to a notification.
func unpaged(p *Incident) bool {
	return p.Tier == Page && !p.rejectionSeen && idleWebhook(p)
}

// settledIdle reports an idle webhook incident the digest carries since
// its announcement.
func settledIdle(p *Incident) bool {
	return p.Delivery.Demoted() && idleWebhook(p)
}

// fixedTier is the tier of an incident that its members do not decide:
// one without members keeps its tier, a drain follows its envelope, and
// an idle webhook the digest carries stays in the digest.
func fixedTier(p *Incident) (Tier, bool) {
	switch {
	case len(p.Members) == 0:
		return p.Tier, true
	case drainingRoot(p):
		return drainTier(p), true
	case settledIdle(p):
		return Digest, true
	}
	return 0, false
}
