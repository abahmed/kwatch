package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
	providercatalog "github.com/abahmed/kwatch/internal/provider/catalog"
)

func alertsOf(names ...string) *config.Config {
	cfg := config.DefaultConfig()
	cfg.Alert = map[string]map[string]interface{}{}
	for _, name := range names {
		cfg.Alert[name] = map[string]interface{}{
			"webhook": "https://example.test/" + name,
			"url":     "https://example.test/" + name,
		}
	}
	return cfg
}

func keptBy(cfg *config.Config) []string {
	kept, _ := capProviders(feature.CommunityEntitlements(), cfg)
	return kept
}

// Alphabetical order kept email over PagerDuty. A pager outranks chat,
// chat outranks email, and the name only breaks a tie.
func TestCommunityKeepsAPagerOverChatOverEmail(t *testing.T) {
	assert.Equal(t, []string{"pagerduty"},
		keptBy(alertsOf("discord", "pagerduty", "slack")))
	assert.Equal(t, []string{"slack"},
		keptBy(alertsOf("email", "slack", "ses")))
	assert.Equal(t, []string{"discord"},
		keptBy(alertsOf("slack", "discord")), "name breaks a tie")
	assert.Equal(t, []string{"email"}, keptBy(alertsOf("email")))
}

func TestCommunityProviderChoiceBeatsTheRank(t *testing.T) {
	cfg := alertsOf("slack", "pagerduty")
	cfg.App.CommunityProvider = "slack"
	assert.Equal(t, []string{"slack"}, keptBy(cfg))
}

func TestCommunityChoiceDoesNotDependOnMapOrder(t *testing.T) {
	for range 50 {
		got := keptBy(alertsOf("email", "slack", "opsgenie", "discord"))
		require.Equal(t, []string{"opsgenie"}, got)
	}
}

func TestEveryRankedProviderIsAKnownProvider(t *testing.T) {
	known := map[string]bool{}
	for _, name := range providercatalog.Names() {
		known[name] = true
	}
	for name := range pagerProviders {
		assert.True(t, known[name], "pager %q", name)
	}
	for name := range emailProviders {
		assert.True(t, known[name], "email %q", name)
	}
}

func TestDroppedPagersAreNamedAndWarned(t *testing.T) {
	out := captureKlog(t)
	cfg := alertsOf("slack", "pagerduty", "opsgenie")
	cfg.App.CommunityProvider = "slack"
	n := newEditionNotice(feature.CommunityEntitlements(), cfg)

	assert.Equal(t, []string{"opsgenie", "pagerduty"}, n.droppedPagers)
	logDroppedPagers(n)
	klog.Flush()

	assert.Equal(t, 1, distinctLines(out.String(), "nothing is paged through"))
	assert.Contains(t, out.String(), "opsgenie, pagerduty")
	assert.Contains(t, out.String(), "app.communityProvider")

	out.Reset()
	logDroppedPagers(newEditionNotice(feature.CommunityEntitlements(),
		alertsOf("pagerduty")))
	klog.Flush()
	assert.Empty(t, out.String(), "the kept pager pages: nothing to warn")
}

func TestProRunsEveryProviderAndWarnsAboutNone(t *testing.T) {
	n := newEditionNotice(feature.ProEntitlements(),
		alertsOf("slack", "pagerduty"))
	assert.Empty(t, n.droppedPagers)
}
