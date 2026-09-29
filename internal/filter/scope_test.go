package filter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func newScope(
	t *testing.T, cfg config.Config, rules ...config.SilenceRule,
) *Scope {
	t.Helper()
	scope, err := NewScope(config.RuntimeConfigFor(&cfg).Scope(),
		rules, false, nil)
	require.NoError(t, err)
	return scope
}

func containerSignal(ns, pod, container, reason string) signal.Signal {
	return signal.Signal{
		Entity: kube.ContainerID(ns, pod, container), Reason: reason,
		Summary: "back-off restarting failed container",
	}
}

func modelWithNamespace(name, labels string) *knowledge.Model {
	model := knowledge.NewModel(knowledge.Options{})
	_, _ = model.Apply(knowledge.Fact{
		Kind: knowledge.Observed, Source: "test", At: time.Now(),
		Entity: knowledge.NewEntityID(kube.KindNamespace, "", name),
		Attributes: map[string]knowledge.Value{
			kube.AttrLabels: knowledge.Text(labels),
		},
	})
	return model
}

func TestScopeNamespacesAndReasons(t *testing.T) {
	scope := newScope(t, config.Config{
		AllowedNamespaces: []string{"shop", "pay"},
		ForbiddenReasons:  []string{"Evicted"},
	})
	model := knowledge.NewModel(knowledge.Options{})

	assert.True(t, scope.Allows(model,
		containerSignal("shop", "api-1", "app", "CrashLoopBackOff")))
	assert.False(t, scope.Allows(model,
		containerSignal("dev", "api-1", "app", "CrashLoopBackOff")))
	assert.False(t, scope.Allows(model,
		containerSignal("shop", "api-1", "app", "Evicted")))
	assert.True(t, scope.Allows(model, signal.Signal{
		Entity: knowledge.NewEntityID(kube.KindNode, "", "n1"),
		Reason: "NodeNotReady",
	}), "cluster-scoped signals ignore namespace scope")
}

func TestScopeNamespaceSelector(t *testing.T) {
	scope := newScope(t, config.Config{NamespaceSelector: "team=shop"})
	sig := containerSignal("shop", "api-1", "app", "OOMKilled")

	assert.True(t, scope.Allows(modelWithNamespace("shop", "team=shop"), sig))
	assert.False(t, scope.Allows(modelWithNamespace("shop", "team=ops"), sig))
	assert.False(t, scope.Allows(
		knowledge.NewModel(knowledge.Options{}), sig),
		"an unknown namespace does not match a selector")
}

func TestScopeSilenceRequiresEveryField(t *testing.T) {
	scope := newScope(t, config.Config{}, config.SilenceRule{
		Namespaces:      []string{"batch"},
		PodNamePatterns: []string{"^report-"},
	})
	model := knowledge.NewModel(knowledge.Options{})

	assert.False(t, scope.Allows(model,
		containerSignal("batch", "report-1", "job", "Error")))
	assert.True(t, scope.Allows(model,
		containerSignal("batch", "api-1", "job", "Error")))
	assert.True(t, scope.Allows(model,
		containerSignal("shop", "report-1", "job", "Error")))
}

func TestScopeSilenceMatchers(t *testing.T) {
	model := knowledge.NewModel(knowledge.Options{})
	node := signal.Signal{
		Entity: knowledge.NewEntityID(kube.KindNode, "", "n1"),
		Reason: "NodeNotReady", Summary: "kubelet stopped posting",
	}
	for _, tc := range []struct {
		name    string
		rule    config.SilenceRule
		sig     signal.Signal
		allowed bool
	}{
		{"container name", config.SilenceRule{
			ContainerNames: []string{"sidecar"}},
			containerSignal("a", "p", "sidecar", "Error"), false},
		{"other container", config.SilenceRule{
			ContainerNames: []string{"sidecar"}},
			containerSignal("a", "p", "app", "Error"), true},
		{"container message", config.SilenceRule{
			ContainerMessages: []string{"back-off"}},
			containerSignal("a", "p", "app", "Error"), false},
		{"node reason", config.SilenceRule{
			NodeReasons: []string{"NodeNotReady"}}, node, false},
		{"node reason on pod", config.SilenceRule{
			NodeReasons: []string{"Error"}},
			containerSignal("a", "p", "app", "Error"), true},
		{"node message", config.SilenceRule{
			NodeMessages: []string{"stopped posting"}}, node, false},
		{"event message", config.SilenceRule{
			EventMessages: []string{"restarting"}},
			containerSignal("a", "p", "app", "Error"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope := newScope(t, config.Config{}, tc.rule)
			assert.Equal(t, tc.allowed, scope.Allows(model, tc.sig))
		})
	}
}

func TestNewScopeRejectsInvalidPolicy(t *testing.T) {
	cfg := config.Config{}
	_, err := NewScope(config.RuntimeConfigFor(&cfg).Scope(),
		[]config.SilenceRule{{PodNamePatterns: []string{"("}}},
		false, nil)
	assert.Error(t, err)

	cfg.NamespaceSelector = "app in ("
	_, err = NewScope(config.RuntimeConfigFor(&cfg).Scope(),
		nil, false, nil)
	assert.Error(t, err)

	var nilScope *Scope
	assert.True(t, nilScope.Allows(nil, signal.Signal{}))
}
