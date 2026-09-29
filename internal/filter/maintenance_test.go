package filter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

var maintenanceNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func maintenanceScope(t *testing.T, enabled bool) *Scope {
	t.Helper()
	cfg := config.Config{}
	scope, err := NewScope(config.RuntimeConfigFor(&cfg).Scope(), nil,
		enabled, func() time.Time { return maintenanceNow })
	require.NoError(t, err)
	return scope
}

func observe(
	m *knowledge.Model, id knowledge.EntityID,
	attrs map[string]knowledge.Value,
) {
	_, _ = m.Apply(knowledge.Fact{
		Kind: knowledge.Observed, Source: "test", At: maintenanceNow,
		Entity: id, Attributes: attrs,
	})
}

func held(value string) map[string]knowledge.Value {
	return map[string]knowledge.Value{
		kube.AttrMaintenance: knowledge.Text(value)}
}

func until(at time.Time) map[string]knowledge.Value {
	return map[string]knowledge.Value{
		kube.AttrMaintenanceUntil: knowledge.Time(at)}
}

func TestScopeMaintenanceHolds(t *testing.T) {
	podID := knowledge.NewEntityID(kube.KindPod, "app", "web")
	nsID := knowledge.NewEntityID(kube.KindNamespace, "", "app")
	cases := []struct {
		name    string
		enabled bool
		id      knowledge.EntityID
		attrs   map[string]knowledge.Value
		want    bool
	}{
		{"entity_on", true, podID, held("true"), false},
		{"entity_false_value", true, podID, held("false"), true},
		{"namespace_on", true, nsID, held("true"), false},
		{"until_future", true, podID,
			until(maintenanceNow.Add(time.Hour)), false},
		{"until_past", true, podID,
			until(maintenanceNow.Add(-time.Hour)), true},
		{"disabled", false, podID, held("true"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := knowledge.NewModel(knowledge.Options{})
			observe(model, tc.id, tc.attrs)
			sig := containerSignal("app", "web", "c", "CrashLoopBackOff")
			sig.Entity = podID
			got := maintenanceScope(t, tc.enabled).Allows(model, sig)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestScopeMaintenanceOwningPodBlocksContainer(t *testing.T) {
	model := knowledge.NewModel(knowledge.Options{})
	observe(model, knowledge.NewEntityID(kube.KindPod, "app", "web"),
		held("true"))
	sig := containerSignal("app", "web", "c", "CrashLoopBackOff")
	assert.False(t, maintenanceScope(t, true).Allows(model, sig))
	other := containerSignal("app", "api", "c", "CrashLoopBackOff")
	assert.True(t, maintenanceScope(t, true).Allows(model, other))
}
