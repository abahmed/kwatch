package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// TestCauseCategoriesCoverEveryRow keeps the category table complete: a
// new explain row must say which category a Community message tells.
func TestCauseCategoriesCoverEveryRow(t *testing.T) {
	for _, row := range explain.Table() {
		if _, ok := causeCategories[row.Name]; !ok {
			t.Errorf("row %q has no entry in causeCategories", row.Name)
		}
	}
	for rule := range causeWords {
		if _, ok := causeCategories[rule]; !ok && rule != "self" {
			t.Errorf("rule %q has no entry in causeCategories", rule)
		}
	}
}

func TestCauseCategoryFollowsTheRowAndMode(t *testing.T) {
	cases := map[string]struct {
		cause *rootcause.CauseRecord
		want  string
	}{
		"no cause":   {nil, ""},
		"on its own": {&rootcause.CauseRecord{Rule: "self"}, ""},
		"missing secret": {&rootcause.CauseRecord{
			Rule: "config-missing-or-changed", Mode: explain.ModeMissing},
			categoryMissingConfig},
		"changed config map": {&rootcause.CauseRecord{
			Rule: "configmap-missing-or-changed", Mode: explain.ModeChanged},
			categoryConfigChange},
		"rollout":       {&rootcause.CauseRecord{Rule: "rollout"}, "a rollout"},
		"unknown row":   {&rootcause.CauseRecord{Rule: "new-row"}, categoryOther},
		"no row at all": {&rootcause.CauseRecord{Summary: "x"}, categoryOther},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, c.want, causeCategory(c.cause))
		})
	}
}

// missingSecretCrash is the payments crash blamed on a missing Secret.
func missingSecretCrash(score float64) incident.Incident {
	p := badRollout()
	secret := inventory.CoreID("secret", "shop", "stripe-api")
	cause := *p.Cause
	cause.Root, cause.Rule, cause.Mode = secret, "config-missing-or-changed",
		explain.ModeMissing
	cause.Score, cause.Change, cause.Summary = score, nil, ""
	cause.Chain = []inventory.EntityID{secret, payments, paymentsCt}
	p.Cause = &cause
	return p
}

func TestCommunityTellsOnlyTheCauseCategory(t *testing.T) {
	full, community, _ := communityWriters()
	d := announce(missingSecretCrash(0.95))
	pro := full.Write(d, at(5, 0))
	require.Contains(t, pro.Note, "stripe-api")

	got := community.Write(d, at(5, 0))
	assert.Contains(t, got.Note,
		"Cause found: a missing Secret or ConfigMap.")
	assert.NotContains(t, got.Note, "stripe-api")
	assert.NotContains(t, got.Title, "because")
	assert.Equal(t, proWhich+proFixTail+proInvite, noteFooter(got.Note))
	assert.Equal(t, 1, strings.Count(got.Note, "Pro shows"))
}

func TestCommunityTeaserKeepsTheCertainty(t *testing.T) {
	_, community, _ := communityWriters()
	got := community.Write(announce(missingSecretCrash(0.6)), at(5, 0))
	assert.Contains(t, got.Note,
		"Cause found: likely a missing Secret or ConfigMap.")
}

// app.hideProHints removes only the Pro line: the category is the free
// signal and stays.
func TestCommunityHiddenHintsKeepTheCategory(t *testing.T) {
	_, community, quiet := communityWriters()
	d := announce(missingSecretCrash(0.95))
	shown := community.Write(d, at(5, 0))
	hidden := quiet.Write(d, at(5, 0))
	assert.Contains(t, hidden.Note, teaserLabel)
	assert.NotContains(t, hidden.Note, "Pro shows")
	assert.Equal(t,
		strings.TrimSuffix(shown.Note, "\n"+noteFooter(shown.Note)),
		hidden.Note)
}

func TestCommunityRevisedCauseLabelsTheCategory(t *testing.T) {
	_, community, _ := communityWriters()
	d := announce(missingSecretCrash(0.95))
	d.Action, d.Reason = incident.Update, incident.ReasonCauseRevised
	got := community.Write(d, at(6, 0))
	assert.Contains(t, got.Note,
		"Cause now known: a missing Secret or ConfigMap.")
	assert.NotContains(t, got.Note, teaserLabel)
	assert.NotContains(t, got.Note, "stripe-api")
}

func TestCommunityListsTellNoCause(t *testing.T) {
	full, community, _ := communityWriters()
	ds := []incident.Decision{announce(missingSecretCrash(0.95))}
	require.Contains(t, full.Rollup(ds, at(6, 0)).Note, "stripe-api")
	got := community.Rollup(ds, at(6, 0))
	assert.NotContains(t, got.Note, "stripe-api")
	assert.NotContains(t, got.Note, "because")
}
