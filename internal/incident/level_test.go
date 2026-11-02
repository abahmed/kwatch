package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestAlertLevelTextCoversEveryCode(t *testing.T) {
	codes := []string{levelPageHeld, levelNotify, levelDigestOnly,
		levelRoutine, levelKnown, levelRhythm, levelCauseOnly,
		levelAdvisory, levelDrain, levelIdle, levelKept, levelLowered,
		levelLoweredTo, levelSilenced, levelOverride, levelNoPaging}
	for _, code := range codes {
		assert.NotEmpty(t, AlertLevelText(code), code)
	}
	for _, rule := range pageRules {
		assert.Contains(t, pageTexts, rule.name, "page rule %s", rule.name)
	}
	assert.Equal(t, "Pages: it matches a rule for failures that need "+
		"someone now.", AlertLevelText(levelPagePrefix+"new-rule"))
	assert.Empty(t, AlertLevelText(""))
	assert.Empty(t, AlertLevelText("unknown"))
}

func TestTierWhyMatchesTier(t *testing.T) {
	notify := sig(entity(kube.KindPod, "web"), reasons.CrashLoopBackOff,
		detection.Warning)
	info := sig(entity(kube.KindPod, "web"), reasons.CrashLoopBackOff,
		detection.Info)
	tests := []struct {
		name     string
		member   detection.Finding
		wantTier Tier
		wantWhy  string
	}{
		{"notify", notify, Notify, levelNotify},
		{"informational", info, Digest, levelDigestOnly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Incident{Root: tt.member.Entity, Opened: t0,
				Members: map[detection.Key]detection.Finding{
					tt.member.Key(): tt.member}}
			got, why := tierWhy(p)
			assert.Equal(t, tt.wantTier, got)
			assert.Equal(t, tt.wantTier, tier(p))
			assert.Equal(t, tt.wantWhy, why)
		})
	}
}

func TestLevelForNamesWhatMovedTheTier(t *testing.T) {
	m := NewManager(Config{IDNonce: testNonce}, nil)
	tests := []struct {
		name             string
		tier, overridden Tier
		base             Tier
		noPage           bool
		want             string
	}{
		{"policy", Notify, Notify, Notify, false, levelNotify},
		{"ratchet", Page, Notify, Notify, false, levelKept},
		{"lowered", Digest, Notify, Notify, false, levelLowered},
		{"lowered to notify", Notify, Page, Page, false, levelLoweredTo},
		{"lowered to silent", Silent, Notify, Notify, false,
			levelSilenced},
		{"override", Page, Page, Notify, false, levelOverride},
		{"paging off", Notify, Notify, Page, true, levelNoPaging},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.override.noPage = tt.noPage
			p := &Incident{Tier: tt.tier}
			assert.Equal(t, tt.want,
				m.levelFor(p, tt.base, levelNotify, tt.overridden))
		})
	}
}
