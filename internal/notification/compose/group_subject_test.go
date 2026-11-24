package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A group whose cause is a Service with no endpoints is titled after
// what that cause reaches, the webhooks calling it, even when an
// unrelated object of another app (another namespace's expired TLS
// Secret) was grouped beside them with a more severe finding.
func TestTitleNamesOnlyTheGroupsOwnObjects(t *testing.T) {
	secret := inventory.CoreID(kube.KindSecret, "kyverno", "kyverno-tls")
	svc := inventory.CoreID(kube.KindService, "otel",
		"otel-operator-webhook")
	hooks := []inventory.EntityID{
		inventory.CoreID(kube.KindMutatingWebhook, "", "otel-mutating"),
		inventory.CoreID(kube.KindValidatingHook, "", "otel-validating"),
	}
	var found []detection.Finding
	for _, h := range hooks {
		found = append(found, detection.Finding{Entity: h,
			Reason: reasons.WebhookNoEndpoints, Severity: detection.Critical,
			Since: writerNow.Add(-10 * time.Minute),
			Summary: "Webhook calls service otel-operator-webhook, " +
				"which has no endpoints"})
	}
	found = append(found, detection.Finding{Entity: secret,
		Reason: reasons.TLSCertExpired, Severity: detection.Critical,
		Since:   writerNow.Add(-time.Hour),
		Summary: "TLS certificate expired 3 days ago"})
	p := incident.Incident{
		ID: "grp-1", Root: svc, Tier: incident.Page,
		State: incident.Open, Opened: writerNow.Add(-10 * time.Minute),
		Members: members(found...),
		Cause: &rootcause.CauseRecord{Root: svc, Score: 0.95,
			Rule:  "webhook-backend-down",
			Chain: []inventory.EntityID{svc, hooks[0]},
			RootFindings: []detection.Finding{{Entity: svc,
				Reason:   reasons.ServiceNoEndpoints,
				Severity: detection.Critical,
				Summary:  "Service has no endpoints"}}},
	}
	msg := Writer{}.Write(announce(p), writerNow)
	assert.NotContains(t, msg.Title, "kyverno")
	assert.Contains(t, msg.Title, "otel-")
}
