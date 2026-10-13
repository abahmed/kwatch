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
)

// An ongoing line says what is wrong, not only that something is, and
// counts from when the problem started, in months or years when long.
func TestOngoingLineSaysWhatIsFailing(t *testing.T) {
	svc := inventory.CoreID(kube.KindService, "kube-addons", "ingress")
	deleting := time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC)
	finding := detection.Finding{Entity: svc, Reason: reasons.StuckDeleting,
		Mode: detection.ModeStuckDeleting, Severity: detection.Warning,
		Since:   deleting,
		Summary: "Deletion is blocked by finalizers service.k8s.aws/x",
		Evidence: []detection.Evidence{{Label: detection.EvidenceFinalizers,
			Value: "service.k8s.aws/x"}}}
	p := incident.Incident{ID: "inc-1", Root: svc, Tier: incident.Digest,
		State: incident.Open, Opened: ongoingNow, Members: members(finding)}

	got := Writer{}.OngoingTitle(p, ongoingNow)

	assert.Contains(t, got, "stuck deleting", "what is failing")
	assert.Contains(t, got, "for five months now.", "age")
	assert.NotContains(t, got, "weeks")
}

// A stuck VolumeAttachment is named by volume and node everywhere, never
// by its hash.
func TestAttachmentNamedByVolumeInReminderAndResolve(t *testing.T) {
	f := attachmentFacts("volume")
	f.now = time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	f.p.Opened = time.Date(2024, 12, 18, 9, 0, 0, 0, time.UTC)
	f.p.Tier = incident.Digest

	reminder := reminderSentences(f)[0].text
	assert.Contains(t, reminder, "ttachment of volume pvc-1234 to node")
	assert.Contains(t, reminder, "about two years now")
	assert.NotContains(t, reminder, "csi-9f8e7d")

	assert.Equal(t, "attachment of volume pvc-1234 to node ip-10-0-1-5 "+
		"(prod-eu-1)", f.leadName(f.p.Root))
	assert.Equal(t, "volume attachment csi-9f8e7d",
		shortName(f.p.Root))
}

// Three held objects are three, are named, and carry the year.
func TestFinalizerLeadCountsAllAndNamesFewWithYear(t *testing.T) {
	f := heldFacts(unhandledCause(""), "arc-system", 3, "x.io/cleanup")
	f.now = time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	for i := range f.members {
		f.members[i].Since = time.Date(2025, 12, 4, 11, 59, 0, 0, time.UTC)
	}

	got := leadSentences(f)[0].text

	assert.Contains(t, got, "Three objects (secret old-0, secret old-1 "+
		"and secret old-2) in arc-system (prod-eu-1) have been stuck "+
		"deleting since Dec 4, 2025")
}
