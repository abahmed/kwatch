package compose

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

func leftoverDecision(i int, sev detection.Severity) incident.Decision {
	root := inventory.CoreID(kube.KindSecret, "ns"+strconv.Itoa(i), "tls")
	f := detection.Finding{Entity: root, Reason: reasons.TLSCertExpired,
		Severity: sev, Summary: "TLS certificate expired on 2026-09-28 " +
			"and nothing in the cluster references it"}
	return incident.Decision{Action: incident.Announce,
		Incident: incident.Incident{ID: root.Namespace, Root: root,
			Tier: incident.Digest, State: incident.Open,
			Members: map[detection.Key]detection.Finding{f.Key(): f}}}
}

func leftoverOngoing(i int) Ongoing {
	return Ongoing{Leftover: reasons.TLSCertExpired,
		Root: inventory.CoreID(kube.KindSecret, "old"+strconv.Itoa(i), "tls")}
}

func TestDigestGroupsExpiredUnusedSecrets(t *testing.T) {
	var opened []incident.Decision
	for i := range 5 {
		opened = append(opened, leftoverDecision(i, detection.Warning))
	}
	msg := Writer{}.DigestWith(opened, nil, nil, DigestExtras{}, extrasNow)

	text := msg.Render(notification.SlackDialect())
	assert.Contains(t, text, "• 5 expired TLS Secrets that nothing "+
		"references: ns0/tls, ns1/tls, ns2/tls +2")
	assert.NotContains(t, text, "Secret *tls*")
	assert.Contains(t, msg.Note, "5 expired TLS Secrets that nothing "+
		"references: ns0/tls, ns1/tls, ns2/tls +2.")
	assert.Contains(t, msg.Note, "five low-priority problems")
}

func TestDigestKeepsALoneLeftoverAndCriticalOnes(t *testing.T) {
	lone := Writer{}.DigestWith(
		[]incident.Decision{leftoverDecision(0, detection.Warning)},
		nil, nil, DigestExtras{}, extrasNow)
	assert.NotContains(t, lone.Note, "expired TLS Secrets")

	critical := Writer{}.DigestWith([]incident.Decision{
		leftoverDecision(0, detection.Critical),
		leftoverDecision(1, detection.Critical)},
		nil, nil, DigestExtras{}, extrasNow)
	assert.NotContains(t, critical.Note, "expired TLS Secrets")
}

func TestDigestGroupsOngoingAndUnchangedLeftovers(t *testing.T) {
	extras := DigestExtras{
		Ongoing:   []Ongoing{leftoverOngoing(1), leftoverOngoing(2)},
		Unchanged: []Ongoing{leftoverOngoing(3), leftoverOngoing(4)},
	}
	msg := Writer{}.DigestWith([]incident.Decision{
		leftoverDecision(0, detection.Warning)}, nil, nil, extras,
		extrasNow)

	text := msg.Render(notification.SlackDialect())
	assert.Contains(t, text, "• 3 expired TLS Secrets that nothing "+
		"references: ns0/tls, old1/tls, old2/tls")
	assert.Contains(t, text, "• 2 expired TLS Secrets that nothing "+
		"references, unchanged: old3/tls, old4/tls")
}

func TestLeftoverNeedsAnOptedInReason(t *testing.T) {
	d := leftoverDecision(0, detection.Warning)
	assert.Equal(t, reasons.TLSCertExpired, LeftoverReason(d.Incident))
	other := leftoverDecision(1, detection.Warning)
	for k, f := range other.Incident.Members {
		f.Reason = reasons.HPAInvalidSelector
		other.Incident.Members[k] = f
	}
	assert.Empty(t, LeftoverReason(other.Incident))
}
