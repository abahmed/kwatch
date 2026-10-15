package app

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/feature"
)

func TestCommunityListsTheProDetectorFamiliesItDoesNotRun(t *testing.T) {
	got := proDetectorsOff(feature.CommunityEntitlements())

	assert.Equal(t, "workloads", got[0])
	assert.Contains(t, got, "storage")
	assert.Contains(t, got, "certificates")
	assert.NotContains(t, got, "active probes")
	assert.Empty(t, proDetectorsOff(feature.ProEntitlements()))
	assert.Equal(t, got, proDetectorsOff(nil), "nil fails closed")
}

// Every Pro detector capability of the catalog must be in a family, or a
// new detector would drop out of Community without a notice.
func TestEveryProDetectorCapabilityBelongsToAFamily(t *testing.T) {
	covered := map[feature.ID]bool{feature.ActiveProbes: true}
	for _, family := range proDetectorFamilies {
		for _, id := range family.ids {
			covered[id] = true
		}
	}
	for _, def := range feature.Catalog() {
		if !strings.HasPrefix(string(def.ID), "core.") ||
			feature.TierOf(def.ID) != feature.Pro {
			continue
		}
		assert.True(t, covered[def.ID], "%s has no detector family", def.ID)
	}
}

func TestCommunityPrefersTheProviderTheOperatorNamed(t *testing.T) {
	cfg := deliveryConfig()
	community := feature.CommunityEntitlements()

	cfg.App.CommunityProvider = "webhook"
	kept, dropped := capProviders(community, cfg)
	assert.Equal(t, []string{"webhook"}, kept)
	assert.Equal(t, []string{"discord", "slack"}, dropped)
	assert.Equal(t, "webhook (inactive: discord, slack)",
		communitySends(community, cfg))

	got := restrictConfig(cfg, community)
	assert.Contains(t, got.Alert, "webhook")
	assert.Len(t, got.Alert, 1)
}

func TestCommunityFallsBackWhenTheNamedProviderIsNotConfigured(t *testing.T) {
	cfg := deliveryConfig()
	cfg.App.CommunityProvider = "pagerduty"

	kept, dropped := capProviders(feature.CommunityEntitlements(), cfg)

	assert.Equal(t, []string{"discord"}, kept, "first by sorted name")
	assert.Equal(t, []string{"slack", "webhook"}, dropped)
	cfg.App.CommunityProvider = ""
	again, _ := capProviders(feature.CommunityEntitlements(), cfg)
	assert.Equal(t, kept, again)
}

func TestCommunitySendsIsEmptyWithoutACapAndForPro(t *testing.T) {
	cfg := deliveryConfig()
	assert.Empty(t, communitySends(feature.ProEntitlements(), cfg))
	delete(cfg.Alert, "slack")
	delete(cfg.Alert, "webhook")
	assert.Empty(t, communitySends(feature.CommunityEntitlements(), cfg))
}

func TestEditionNoticeLinesMatchStatusAndLint(t *testing.T) {
	cfg := deliveryConfig()
	cfg.App.CommunityProvider = "slack"
	lines := newEditionNotice(feature.CommunityEntitlements(), cfg).lines()

	require.Len(t, lines, 2)
	assert.Regexp(t, `^Pro detectors not running: \d+ `+
		`\(workloads, storage, certificates, …\)$`, lines[0])
	assert.Equal(t,
		"Community sends to: slack (inactive: discord, webhook)", lines[1])
	assert.Empty(t, newEditionNotice(
		feature.ProEntitlements(), cfg).lines())
}

type editionRig struct {
	disk diskState
	sent []string
}

func (r *editionRig) start(
	t *testing.T, edition feature.Edition, markInitialized bool,
) {
	t.Helper()
	ctx := context.Background()
	plan, err := planEdition(ctx, r.disk, entitlementsOf(edition))
	require.NoError(t, err)
	if markInitialized {
		require.NoError(t, r.disk.MarkAsInitialized(ctx, "cluster", "v1"))
	}
	plan.commit(r.disk, func(m string) { r.sent = append(r.sent, m) }, "note")
}

func newEditionRig(t *testing.T) *editionRig {
	return &editionRig{disk: diskState{store: openTestStore(t)}}
}

func TestFreshInstallSendsNoEditionNotice(t *testing.T) {
	r := newEditionRig(t)

	r.start(t, feature.CommunityEdition, true)
	r.start(t, feature.CommunityEdition, true)

	assert.Empty(t, r.sent)
}

func TestUpgradeFromAnEarlierVersionSendsOneNotice(t *testing.T) {
	r := newEditionRig(t)
	// An earlier version initialized the state and knew no edition.
	require.NoError(t, r.disk.MarkAsInitialized(
		context.Background(), "cluster", "v0"))

	for range 3 {
		r.start(t, feature.CommunityEdition, true)
	}

	assert.Equal(t, []string{"note"}, r.sent, "once across restarts")
}

func TestMovingFromProToCommunitySendsOneNotice(t *testing.T) {
	r := newEditionRig(t)
	r.start(t, feature.ProEdition, true)
	r.start(t, feature.ProEdition, true)
	assert.Empty(t, r.sent)

	for range 3 {
		r.start(t, feature.CommunityEdition, true)
	}
	assert.Len(t, r.sent, 1)

	// Pro again and back to Community is a new move, told once more.
	r.start(t, feature.ProEdition, true)
	r.start(t, feature.CommunityEdition, true)
	r.start(t, feature.CommunityEdition, true)
	assert.Len(t, r.sent, 2)
}

func TestEditionIsNotRecordedUntilCommit(t *testing.T) {
	r := newEditionRig(t)
	require.NoError(t, r.disk.MarkAsInitialized(
		context.Background(), "cluster", "v0"))

	_, err := planEdition(context.Background(), r.disk,
		feature.CommunityEntitlements())
	require.NoError(t, err)
	r.start(t, feature.CommunityEdition, false)

	assert.Len(t, r.sent, 1, "a plan that was never committed sent nothing")
}

func TestUpgradeNoticeTextExplainsTheEditionsAndLinksTheTrial(t *testing.T) {
	text := upgradeNoticeText(newEditionNotice(
		feature.CommunityEntitlements(), deliveryConfig()))

	for _, want := range []string{"Community", "pod and node alerts",
		"Pro restores", "Pro detectors not running",
		"https://kwatch.dev/signup?src=upgrade"} {
		assert.Contains(t, text, want)
	}
}

func entitlementsOf(e feature.Edition) feature.Entitlements {
	if e == feature.ProEdition {
		return feature.ProEntitlements()
	}
	return feature.CommunityEntitlements()
}
