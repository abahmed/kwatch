package controller

import (
	"testing"
)

func TestControllerNamespaceAccessorsRespectScope(t *testing.T) {
	controller := &Controller{scopeState: scopeState{
		allowedNamespaces:   makeNamespaceSet([]string{"zeta", "alpha"}),
		forbiddenNamespaces: makeNamespaceSet([]string{"blocked"}),
	}}
	if controller.NamespaceAllowed("blocked") ||
		!controller.NamespaceAllowed("alpha") ||
		controller.NamespaceAllowed("other") {
		t.Fatal("namespace scope decision was incorrect")
	}
	namespaces, all := controller.NamespaceScope()
	if all || len(namespaces) != 2 || namespaces[0] != "alpha" {
		t.Fatalf("namespace scope = %v, all=%v", namespaces, all)
	}
	controller.watchAll = true
	if controller.NamespaceAllowed("blocked") {
		t.Fatal("watch-all scope should still honor forbidden namespaces")
	}
	if namespaces, all = controller.NamespaceScope(); !all || namespaces != nil {
		t.Fatalf("watch-all scope = %v, all=%v", namespaces, all)
	}
}

func TestControllerResourceLookupAliasesAndUnknownKinds(t *testing.T) {
	for _, resource := range []string{
		"pod", "pods", "pvc", "persistentvolumeclaims", "hpa",
		"horizontalpodautoscalers", "ns", "namespaces",
	} {
		if _, ok := lookupExists(resource); !ok {
			t.Fatalf("lookupExists(%q) did not resolve", resource)
		}
	}
	if _, ok := lookupExists("unknown"); ok {
		t.Fatal("unknown resource resolved")
	}
	controller := &Controller{}
	if exists, known := controller.ResourceExists(
		"pod", "apps", "api",
	); exists || known {
		t.Fatal("unavailable pod lister should be unknown")
	}
	if exists, known := controller.ResourceExists(
		"pod", "apps", "",
	); exists || known {
		t.Fatal("empty resource name should be unknown")
	}
}
