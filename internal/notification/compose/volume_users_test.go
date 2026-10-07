package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// fullClaimIncident is a claim whose own finding carries evidence, with
// a crashing container beside it.
func fullClaimIncident(claimEvidence []detection.Evidence, reason string,
	crash string) incident.Incident {
	claim := inventory.CoreID(kube.KindPVC, "inventory", "pgdata")
	ct := inventory.CoreID(kube.KindContainer, "inventory", "db-4d/app")
	return incident.Incident{
		ID: "inc-10", Root: claim, Tier: incident.Notify,
		State: incident.Open, Opened: at(3, 0), Revision: 1,
		Members: members(
			detection.Finding{Entity: claim, Reason: reason,
				Severity: detection.Critical, Since: at(3, 0),
				Summary: "Storage is over 95% used", Evidence: claimEvidence},
			detection.Finding{Entity: ct, Reason: reasons.CrashLoopBackOff,
				Severity: detection.Critical, Since: at(3, 0),
				Summary: "Container is crash looping",
				Evidence: []detection.Evidence{{
					Label: detection.EvidenceError, Value: crash}}}),
	}
}

func TestFullClaimSaysSizeUsersAndTheCrashItCaused(t *testing.T) {
	p := fullClaimIncident([]detection.Evidence{
		{Label: "used", Value: "96%"},
		{Label: detection.EvidenceVolumeSize, Value: "19.2Gi of 20Gi"},
		{Label: detection.EvidenceVolumeUsedBy, Value: "backup, postgres"},
	}, reasons.VolumeUsageHigh,
		"could not write: No space left on device")

	text := notification.Text(Writer{}.Write(announce(p), at(10, 0)))

	assert.Contains(t, text, "It is at 96% (about 19 GiB of 20 GiB).")
	assert.Contains(t, text, "It is used by backup and postgres.")
	assert.Contains(t, text, "fails with \"could not write: No space left "+
		"on device\".")
}

func TestFullClaimIgnoresACrashThatIsNotAboutTheDisk(t *testing.T) {
	p := fullClaimIncident([]detection.Evidence{
		{Label: "used", Value: "96%"}}, reasons.VolumeUsageHigh,
		"panic: nil pointer")

	text := notification.Text(Writer{}.Write(announce(p), at(10, 0)))

	assert.NotContains(t, text, "panic: nil pointer")
}

func TestFullInodesAreSaidApartFromBytes(t *testing.T) {
	p := fullClaimIncident([]detection.Evidence{
		{Label: detection.EvidenceVolumeInodes, Value: "99%"},
		{Label: detection.EvidenceVolumeSize, Value: "12Gi of 100Gi"},
	}, reasons.VolumeInodesHigh, "write failed: no space left on device")

	text := notification.Text(Writer{}.Write(announce(p), at(10, 0)))

	assert.Contains(t, text, "Its inodes are 99% used, its space 12 GiB "+
		"of 100 GiB.")
	assert.NotContains(t, text, "It is at 99%")
}

func TestAndListJoinsNames(t *testing.T) {
	assert.Equal(t, "a", andList("a"))
	assert.Equal(t, "a and b", andList("a, b"))
	assert.Equal(t, "a, b and c", andList("a, b, c"))
	assert.Equal(t, "a, b, c +2 more", andList("a, b, c +2 more"))
}
