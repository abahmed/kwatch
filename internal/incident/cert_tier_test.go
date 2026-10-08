package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An expired certificate nothing references is a warning that waits for
// the digest; one in use is critical and interrupts.
func TestTierExpiredCertificateFollowsUse(t *testing.T) {
	secret := entity(kube.KindSecret, "old-tls")
	unused := incidentOf(secret, nil,
		sig(secret, reasons.TLSCertExpired, detection.Warning))
	used := incidentOf(secret, nil,
		sig(secret, reasons.TLSCertExpired, detection.Critical))

	assert.Equal(t, Digest, tier(unused))
	assert.NotEqual(t, Digest, tier(used))
}

// A restored Notify incident for an expired certificate that is now
// found unused falls to the digest once the restore grace is over.
func TestManagerRestoredExpiredCertificateLowersWhenUnused(t *testing.T) {
	secret := entity(kube.KindSecret, "old-tls")
	src := newRig(t, Config{})
	announced(t, src, sig(secret, reasons.TLSCertExpired,
		detection.Critical))
	require.Equal(t, Notify, src.only().Tier)

	dst := newRig(t, Config{})
	grace := at(10 * time.Minute)
	dst.m.Restore(src.m.Export(), grace)
	dst.raise(at(time.Minute), sig(secret, reasons.TLSCertExpired,
		detection.Warning))
	dst.tick(grace.Add(time.Second))
	dst.tick(grace.Add(time.Second + DefaultReviseSettle))

	assert.Equal(t, Digest, dst.only().Tier)
}
