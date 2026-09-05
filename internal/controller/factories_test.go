package controller

import (
	"testing"

	"k8s.io/client-go/kubernetes/fake"
)

func TestNewFactoriesMultiNamespaceIncludesClusterScope(t *testing.T) {
	client := fake.NewSimpleClientset()
	scope := namespaceScope{namespaces: []string{"one", "two"}}
	fs, factories := newFactories(client, scope, nil, 0)

	if fs.clusterScoped == nil {
		t.Fatal("multi-namespace scope must include a cluster-scoped factory")
	}
	if len(fs.perNamespace) != 2 {
		t.Fatalf("expected 2 namespace factories, got %d", len(fs.perNamespace))
	}
	if len(factories) != 3 {
		t.Fatalf(
			"expected 3 factories including cluster scope, got %d",
			len(factories),
		)
	}
}
