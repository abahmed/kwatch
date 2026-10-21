package app

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/feature"
)

const inactiveWarning = "configuration sets kwatch Pro features"

func communityDeps() *serverDeps {
	return &serverDeps{
		entitlements: feature.CommunityEntitlements(),
		inactivePro:  []string{"policy.severity"},
		notice:       newEditionNotice(feature.CommunityEntitlements(), nil),
	}
}

func TestLogEditionLogsTheInactiveProSettingsOncePerGeneration(t *testing.T) {
	out := captureKlog(t)
	deps := communityDeps()

	deps.logEdition()
	deps.logEdition()
	klog.Flush()

	text := out.String()
	require.Equal(t, 1, distinctLines(text, inactiveWarning), text)
	require.Contains(t, text, "policy.severity")
	require.Contains(t, text, "W", "must be a warning")
}

// With a license key the Community bootstrap is provisional: a Pro start
// would warn for a second about an edition it is about to leave.
func TestGenerationLogsTheEditionLateWhileALicenseKeyIsPending(t *testing.T) {
	out := captureKlog(t)
	h := newGenHarness(t)
	h.tweakBoot = func(b *bootstrap) {
		b.entitlements = feature.CommunityEntitlements()
		b.inactivePro = []string{"policy.severity"}
		b.license = &licenseSetup{key: "KW1-pending"}
	}
	var duringServe, afterSettle string
	h.serve = func(_ int, deps *serverDeps) int {
		klog.Flush()
		duringServe = out.String()
		deps.logEdition() // the leader does this once the edition is final
		klog.Flush()
		afterSettle = out.String()
		return endGeneration(deps, nil)
	}

	require.Zero(t, h.env().runAll(feature.CommunityEntitlements()))

	require.NotContains(t, duringServe, inactiveWarning,
		"the bootstrap must not warn while the license is pending")
	require.Contains(t, afterSettle, inactiveWarning)
}

func TestGenerationLogsTheEditionAtOnceWithoutALicenseKey(t *testing.T) {
	out := captureKlog(t)
	h := newGenHarness(t)
	h.tweakBoot = func(b *bootstrap) {
		b.entitlements = feature.CommunityEntitlements()
		b.inactivePro = []string{"policy.severity"}
	}
	var duringServe string
	h.serve = func(_ int, deps *serverDeps) int {
		klog.Flush()
		duringServe = out.String()
		return endGeneration(deps, nil)
	}

	require.Zero(t, h.env().runAll(feature.CommunityEntitlements()))

	require.Contains(t, duringServe, inactiveWarning)
}
