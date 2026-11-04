package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var guardService = inventory.CoreID(kube.KindService, "guard", "guard-svc")

// reachModel holds a pod labelled app=api in namespace web (labelled
// team=web), and a webhook configuration whose fail-closed rules are
// lines.
func reachModel(
	t *testing.T, lines string,
) (*inventory.Model, inventory.Entity) {
	t.Helper()
	m := inventory.NewModel(inventory.Options{})
	config := inventory.CoreID(kube.KindValidatingHook, "", "guard")
	for id, attrs := range map[inventory.EntityID]map[string]inventory.Value{
		inventory.CoreID(kube.KindNamespace, "", "web"): {
			kube.AttrLabels: inventory.Text("team=web")},
		inventory.CoreID(kube.KindPod, "web", "api-1"): {
			kube.AttrLabels: inventory.Text("app=api")},
		config: {kube.AttrWebhookClosedRules: inventory.Text(lines)},
	} {
		_, err := m.Apply(inventory.Observation{Kind: inventory.Observed,
			Source: kube.ObservationSource, At: fixedTime(), Entity: id,
			Attributes: attrs})
		require.NoError(t, err)
	}
	e, ok := m.Entity(config)
	require.True(t, ok)
	return m, e
}

func allSynced(inventory.Kind) bool { return true }

func reachOf(t *testing.T, lines string) kube.WebhookReach {
	t.Helper()
	m, config := reachModel(t, lines)
	return kube.ClosedRuleReach(m, allSynced, config, guardService)
}

func TestClosedRuleReachFindsResourcesWithObjects(t *testing.T) {
	got := reachOf(t, "guard/guard-svc||pods|*|1||\n"+
		"guard/guard-svc|apps|deployments|*|1||\n"+
		"other/svc||namespaces|*|1||")

	assert.True(t, got.Listed)
	assert.Equal(t, []string{"pods"}, got.Resources)
	assert.False(t, got.Unknown)
}

func TestClosedRuleReachAppliesScopeAndSelectors(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{"namespace selector matches", "||pods|*|1|team=web|",
			[]string{"pods"}},
		{"namespace selector excludes", "||pods|*|1|team=db|", nil},
		{"object selector matches", "||pods|*|1||app=api",
			[]string{"pods"}},
		{"object selector excludes", "||pods|*|1||app!=api", nil},
		{"cluster scope skips pods", "||pods|Cluster|1||", nil},
		{"wildcard", "|*|*|Namespaced|1||", []string{"pods"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := reachOf(t, "guard/guard-svc"+tc.line)

			assert.Equal(t, tc.want, got.Resources)
			assert.False(t, got.Unknown)
		})
	}
}

func TestClosedRuleReachOfGoneCustomResourcesIsNone(t *testing.T) {
	got := reachOf(t, "guard/guard-svc|widgets.example.com|widgets|*|1||")

	assert.True(t, got.None())
}

func TestClosedRuleReachSaysWhenItCannotTell(t *testing.T) {
	tests := map[string]string{
		"custom resource defined": "|gadgets.example.com|gadgets|*|1||",
		"only deletes":            "||pods|*|0||",
		"subresource":             "||pods/status|*|1||",
		"unwatched built-in":      "|coordination.k8s.io|leases|*|1||",
		"labels not held":         "|apps|deployments|*|1||x=y",
	}
	for name, line := range tests {
		t.Run(name, func(t *testing.T) {
			m, config := reachModel(t, "guard/guard-svc"+line)
			_, err := m.Apply(inventory.Observation{
				Kind: inventory.Observed, Source: kube.ObservationSource,
				At: fixedTime(), Entity: inventory.CoreID(kube.KindCRD, "",
					"gadgets.gadgets.example.com")})
			require.NoError(t, err)
			_, err = m.Apply(inventory.Observation{
				Kind: inventory.Observed, Source: kube.ObservationSource,
				At: fixedTime(), Entity: inventory.CoreID(
					kube.KindDeployment, "web", "api")})
			require.NoError(t, err)

			got := kube.ClosedRuleReach(m, allSynced, config, guardService)

			assert.True(t, got.Unknown)
			assert.False(t, got.None())
		})
	}
}

func TestClosedRuleReachOfAnUnsyncedKindIsUnknown(t *testing.T) {
	m, config := reachModel(t, "guard/guard-svc||pods|*|1||")
	synced := func(k inventory.Kind) bool { return k != kube.KindPod }

	got := kube.ClosedRuleReach(m, synced, config, guardService)

	assert.True(t, got.Unknown)
	assert.Empty(t, got.Resources)
}

func TestClosedRuleReachWithoutRulesIsNotListed(t *testing.T) {
	got := reachOf(t, "")

	assert.False(t, got.Listed)
	assert.False(t, got.None())
}
