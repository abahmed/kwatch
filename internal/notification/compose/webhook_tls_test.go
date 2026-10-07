package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

func tlsCase(message string) caseFacts {
	hook := inventory.CoreID(kube.KindValidatingHook, "", "policy")
	return caseFacts{
		p: incident.Incident{Root: hook, Cause: &rootcause.CauseRecord{
			Rule: "webhook-rejects", Mode: explain.ModeWebhookTLS,
			Root: hook,
		}},
		members: []detection.Finding{{
			Summary: "Controller cannot create pods",
			Evidence: []detection.Evidence{
				{Label: "event", Value: message}},
		}},
	}
}

func TestTLSReasonIsTheErrorTheAPIServerQuoted(t *testing.T) {
	f := tlsCase("Error creating: Internal error occurred: failed " +
		"calling webhook \"p.example.com\": failed to call webhook: " +
		"Post \"https://p.ns.svc:443/v?timeout=10s\": tls: failed to " +
		"verify certificate: x509: certificate signed by unknown " +
		"authority")

	assert.Equal(t, "x509: certificate signed by unknown authority",
		tlsReason(f))
}

func TestTLSReasonIsEmptyWithoutATLSError(t *testing.T) {
	assert.Empty(t, tlsReason(tlsCase("context deadline exceeded")))
}

func TestOnlyTLSWebhookCausesGetTheTLSLead(t *testing.T) {
	f := tlsCase("x509: certificate signed by unknown authority")
	f.p.Cause.Mode = explain.ModeWebhookTimeout

	_, ok := tlsLead(f)

	assert.False(t, ok)
}

func TestServingCertSentenceStatesTheExpiryThatHappened(t *testing.T) {
	f := tlsCase("x509: certificate has expired")
	f.p.Cause.ServingCert = &rootcause.ServingCert{
		Secret: inventory.CoreID(kube.KindSecret, "policy-system",
			"policy-tls"),
		Expired: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC),
	}

	got := servingCertSentences(f)

	assert.Len(t, got, 1)
	assert.Equal(t, "Its serving certificate in Secret "+
		"policy-system/policy-tls expired at 2026-10-01 09:00.",
		got[0].text)
	f.p.Cause.ServingCert = nil
	assert.Empty(t, servingCertSentences(f))
}
