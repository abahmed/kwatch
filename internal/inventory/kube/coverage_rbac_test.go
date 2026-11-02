package kube

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCoverageGivesTheRBACRuleOfARefusedKind(t *testing.T) {
	got := BuildCoverage(CoverageInput{
		Unavailable: []SourceStatus{
			{Resource: "secrets", Reason: ReasonPermissionDenied},
			{Resource: "events", Reason: ReasonPermissionDenied},
			{Resource: "leases", Group: "coordination.k8s.io",
				Reason: ReasonSyncPending},
		},
		Dynamic: DynamicStatus{Kinds: []DynamicKind{
			{Resource: "certificates.cert-manager.io",
				Reason: ReasonPermissionDenied},
			{Resource: "widgets.example.com", Reason: ReasonWatchBudget},
		}},
	})

	require.Len(t, got.Kinds.Unreadable, 3)
	cert := got.Kinds.Unreadable[0]
	assert.Equal(t, "certificates.cert-manager.io", cert.Kind)
	assert.Equal(t, &RBACRule{APIGroups: []string{"cert-manager.io"},
		Resources: []string{"certificates"},
		Verbs:     []string{"get", "list", "watch"}}, cert.Rule)
	assert.Nil(t, got.Kinds.Unsynced[0].Rule, "only refused kinds")
	assert.Nil(t, got.Kinds.NotWatched[0].Rule)

	assert.Equal(t, []RBACRule{
		{APIGroups: []string{""}, Resources: []string{"events", "secrets"},
			Verbs: []string{"get", "list", "watch"}},
		{APIGroups: []string{"cert-manager.io"},
			Resources: []string{"certificates"},
			Verbs:     []string{"get", "list", "watch"}},
	}, got.Kinds.MissingRules)

	raw, err := json.Marshal(got.Kinds.MissingRules[0])
	require.NoError(t, err)
	assert.JSONEq(t, `{"apiGroups":[""],"resources":["events","secrets"],`+
		`"verbs":["get","list","watch"]}`, string(raw))
}

func TestBuildCoverageHasNoMissingRulesWhenNothingIsRefused(t *testing.T) {
	got := BuildCoverage(CoverageInput{})
	assert.Nil(t, got.Kinds.MissingRules)
}
