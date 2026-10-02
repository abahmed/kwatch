package scope

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func newScope(
	t *testing.T, cfg config.Config, rules ...config.SilenceRule,
) *Scope {
	t.Helper()
	scope, err := New(config.RuntimeConfigFor(&cfg).Scope(),
		rules, false, nil)
	require.NoError(t, err)
	return scope
}

func containerFinding(ns, pod, container, reason string) detection.Finding {
	return detection.Finding{
		Entity: kube.ContainerID(ns, pod, container), Reason: reason,
		Summary: "back-off restarting failed container",
	}
}

func modelWithNamespace(name, labels string) *inventory.Model {
	model := inventory.NewModel(inventory.Options{})
	_, _ = model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: time.Now(),
		Entity: inventory.CoreID(kube.KindNamespace, "", name),
		Attributes: map[string]inventory.Value{
			kube.AttrLabels: inventory.Text(labels),
		},
	})
	return model
}

func TestScopeNamespacesAndReasons(t *testing.T) {
	scope := newScope(t, config.Config{
		AllowedNamespaces: []string{"shop", "pay"},
		ForbiddenReasons:  []string{"Evicted"},
	})
	model := inventory.NewModel(inventory.Options{})

	assert.True(t, scope.Allows(model,
		containerFinding("shop", "api-1", "app", "CrashLoopBackOff")))
	assert.False(t, scope.Allows(model,
		containerFinding("dev", "api-1", "app", "CrashLoopBackOff")))
	assert.False(t, scope.Allows(model,
		containerFinding("shop", "api-1", "app", "Evicted")))
	assert.True(t, scope.Allows(model, detection.Finding{
		Entity: inventory.CoreID(kube.KindNode, "", "n1"),
		Reason: "NodeNotReady",
	}), "cluster-scoped findings ignore namespace scope")
}

func TestScopeNamespaceSelector(t *testing.T) {
	scope := newScope(t, config.Config{NamespaceSelector: "team=shop"})
	sig := containerFinding("shop", "api-1", "app", "OOMKilled")

	assert.True(t, scope.Allows(modelWithNamespace("shop", "team=shop"), sig))
	assert.False(t, scope.Allows(modelWithNamespace("shop", "team=ops"), sig))
	assert.False(t, scope.Allows(
		inventory.NewModel(inventory.Options{}), sig),
		"an unknown namespace does not match a selector")
}

func TestScopeSilenceRequiresEveryField(t *testing.T) {
	scope := newScope(t, config.Config{}, config.SilenceRule{
		Namespaces:      []string{"batch"},
		PodNamePatterns: []string{"^report-"},
	})
	model := inventory.NewModel(inventory.Options{})

	assert.False(t, scope.Allows(model,
		containerFinding("batch", "report-1", "job", "Error")))
	assert.True(t, scope.Allows(model,
		containerFinding("batch", "api-1", "job", "Error")))
	assert.True(t, scope.Allows(model,
		containerFinding("shop", "report-1", "job", "Error")))
}

func TestScopeSilenceMatchers(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	node := detection.Finding{
		Entity: inventory.CoreID(kube.KindNode, "", "n1"),
		Reason: "NodeNotReady", Summary: "kubelet stopped posting",
	}
	for _, tc := range []struct {
		name    string
		rule    config.SilenceRule
		sig     detection.Finding
		allowed bool
	}{
		{"container name", config.SilenceRule{
			ContainerNames: []string{"sidecar"}},
			containerFinding("a", "p", "sidecar", "Error"), false},
		{"other container", config.SilenceRule{
			ContainerNames: []string{"sidecar"}},
			containerFinding("a", "p", "app", "Error"), true},
		{"container message", config.SilenceRule{
			ContainerMessages: []string{"back-off"}},
			containerFinding("a", "p", "app", "Error"), false},
		{"node reason", config.SilenceRule{
			NodeReasons: []string{"NodeNotReady"}}, node, false},
		{"node reason on pod", config.SilenceRule{
			NodeReasons: []string{"Error"}},
			containerFinding("a", "p", "app", "Error"), true},
		{"node message", config.SilenceRule{
			NodeMessages: []string{"stopped posting"}}, node, false},
		{"event message", config.SilenceRule{
			EventMessages: []string{"restarting"}},
			containerFinding("a", "p", "app", "Error"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope := newScope(t, config.Config{}, tc.rule)
			assert.Equal(t, tc.allowed, scope.Allows(model, tc.sig))
		})
	}
}

func TestScopeNewRejectsInvalidPolicy(t *testing.T) {
	cfg := config.Config{}
	_, err := New(config.RuntimeConfigFor(&cfg).Scope(),
		[]config.SilenceRule{{PodNamePatterns: []string{"("}}},
		false, nil)
	assert.Error(t, err)

	cfg.NamespaceSelector = "app in ("
	_, err = New(config.RuntimeConfigFor(&cfg).Scope(),
		nil, false, nil)
	assert.Error(t, err)

	var nilScope *Scope
	assert.True(t, nilScope.Allows(nil, detection.Finding{}))
}

func TestScopeEmptySilenceRuleMatchesNothing(t *testing.T) {
	scope := newScope(t, config.Config{}, config.SilenceRule{})
	model := inventory.NewModel(inventory.Options{})

	assert.True(t, scope.Allows(model,
		containerFinding("shop", "api-1", "app", "CrashLoopBackOff")))
	assert.True(t, scope.Allows(model, detection.Finding{
		Entity: inventory.CoreID(kube.KindNode, "", "n1"),
		Reason: "NodeNotReady",
	}))
}
