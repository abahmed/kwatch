package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// removedOperator is an incident of the objects a removed policy
// operator left behind, rooted at its gone Service.
func removedOperator(findings ...detection.Finding) incident.Incident {
	backend := inventory.CoreID("service", "policy-system", "policy-svc")
	p := incident.Incident{ID: "left", Root: backend,
		State: incident.Open, Tier: incident.Digest,
		Members: map[detection.Key]detection.Finding{}}
	for _, f := range findings {
		f.Leftover = backend
		p.Members[f.Key()] = f
	}
	return p
}

func leftoverFinding(kind inventory.Kind, ns, name, why string,
) detection.Finding {
	return detection.Finding{Entity: inventory.CoreID(kind, ns, name),
		Reason: why, Severity: detection.Info, Since: revisionNow,
		Summary: why + " on " + name}
}

// The leftovers of one removed controller are titled by it, with the
// count of what it left behind.
func TestLeftoverGroupIsTitledByItsAbsentBackend(t *testing.T) {
	p := removedOperator(
		leftoverFinding("validatingwebhookconfiguration", "",
			"policy-hook", reasons.WebhookBackendNotFound),
		leftoverFinding("secret", "policy-system", "policy-tls",
			reasons.TLSCertExpired),
		leftoverFinding("secret", "policy-system", "cleanup-tls",
			reasons.TLSCertExpired))

	got := NewWriter("", nil).ProblemTitle(p, nil, revisionNow)

	assert.Contains(t, got, "Leftovers of a removed or stopped "+
		"controller (3 objects)")
	assert.Contains(t, got, "policy-svc")
	assert.Contains(t, got, "is gone")
}

// One leftover alone keeps its own lead.
func TestSingleLeftoverKeepsItsOwnLead(t *testing.T) {
	p := removedOperator(leftoverFinding("secret", "policy-system",
		"policy-tls", reasons.TLSCertExpired))

	got := NewWriter("", nil).ProblemTitle(p, nil, revisionNow)

	assert.NotContains(t, got, "Leftovers")
}

// A certificate that joined the incident of a webhook backend only as
// its leftover is not caused by it: no "because" names the backend.
func TestLeftoverBesideACauseIsNotCausedByIt(t *testing.T) {
	p := removedOperator(leftoverFinding("secret", "policy-system",
		"audit-tls", reasons.TLSCertExpired))
	p.Cause = &rootcause.CauseRecord{Root: p.Root, Score: 0.9,
		Rule: "webhook-backend-missing"}

	got := NewWriter("", nil).ProblemTitle(p, nil, revisionNow)

	assert.Contains(t, got, "audit-tls")
	assert.NotContains(t, got, "because")
}

// Of a cause's own failure and a leftover beside it, the cause's own
// failure leads.
func TestLeadPrefersAFailureTheCauseExplains(t *testing.T) {
	root := inventory.CoreID("service", "policy-system", "policy-svc")
	left := leftoverFinding("secret", "policy-system", "audit-tls",
		reasons.TLSCertExpired)
	left.Leftover = root
	hook := leftoverFinding("validatingwebhookconfiguration", "",
		"policy-hook", reasons.WebhookNoEndpoints)

	got := leadFailure(root, []detection.Finding{left, hook})

	assert.Equal(t, hook.Entity, got.Entity)
}
