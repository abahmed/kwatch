package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
)

// startWithConfig is start for a configuration that sets Pro features.
func (r *editionRig) startWithConfig(
	t *testing.T, edition feature.Edition, usesPro bool,
) {
	t.Helper()
	ctx := context.Background()
	plan, err := planEdition(ctx, r.disk, entitlementsOf(edition), usesPro)
	require.NoError(t, err)
	require.NoError(t, r.disk.MarkAsInitialized(ctx, "cluster", "v1"))
	plan.commit(r.disk, func(m string) { r.sent = append(r.sent, m) }, "note")
}

// A first Community start whose configuration sets Pro features is the
// moment those settings silently stop working, so it is told once.
func TestFirstCommunityStartWithProSettingsSendsOneNotice(t *testing.T) {
	r := newEditionRig(t)

	for range 3 {
		r.startWithConfig(t, feature.CommunityEdition, true)
	}

	assert.Equal(t, []string{"note"}, r.sent, "once across restarts")
}

func TestCommunityStartWithoutProSettingsStaysSilent(t *testing.T) {
	r := newEditionRig(t)
	r.startWithConfig(t, feature.CommunityEdition, false)
	r.startWithConfig(t, feature.CommunityEdition, false)
	assert.Empty(t, r.sent)
}

// An operator who adds Pro settings to a running Community install hears
// about it at the next start, once.
func TestProSettingsAddedLaterAreToldOnce(t *testing.T) {
	r := newEditionRig(t)
	r.startWithConfig(t, feature.CommunityEdition, false)
	r.startWithConfig(t, feature.CommunityEdition, true)
	r.startWithConfig(t, feature.CommunityEdition, true)
	assert.Len(t, r.sent, 1)
}

func TestProStartIgnoresProSettings(t *testing.T) {
	r := newEditionRig(t)
	r.startWithConfig(t, feature.ProEdition, true)
	assert.Empty(t, r.sent)
}

func TestNoticeTextsNameTheIgnoredSettings(t *testing.T) {
	cfg := proConfig()
	n := newEditionNotice(feature.CommunityEntitlements(), cfg)
	require.NotEmpty(t, n.inactive)

	for name, text := range map[string]string{
		"upgrade": upgradeNoticeText(n), "config": configNoticeText(n),
	} {
		assert.Contains(t, text, "Your configuration sets Pro features "+
			"that are not running and are ignored", name)
		assert.Contains(t, text, n.inactive[0], name)
		assert.Contains(t, text, upgradeTrialURL, name)
	}
	assert.NotContains(t, configNoticeText(n), "is now running",
		"an install that never ran Pro was not moved")
	clean := newEditionNotice(feature.CommunityEntitlements(),
		config.DefaultConfig())
	assert.NotContains(t, upgradeNoticeText(clean),
		"Your configuration sets")
}
