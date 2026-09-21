package rbac

import (
	"testing"
	"time"
)

func TestSampledNamespaces(t *testing.T) {
	all := sampled([]string{"a", "b"}, true, 0)
	if !all["a"] || !all["b"] {
		t.Fatal("full sweep did not select every namespace")
	}
	for cycle, want := range []string{"a", "b", "a"} {
		selected := sampled([]string{"a", "b"}, false, cycle)
		if len(selected) != 1 || !selected[want] {
			t.Fatalf("cycle %d selected %v, want %s", cycle, selected, want)
		}
	}
	if selected := sampled(nil, false, 0); len(selected) != 0 {
		t.Fatalf("empty selection = %v", selected)
	}
}

func TestSecurityStateAndStatusSnapshot(t *testing.T) {
	if got := securityState(Status{RBACDenied: true}); got != "rbacDenied" {
		t.Fatalf("denied state = %q", got)
	}
	if got := securityState(Status{}); got != "unavailable" {
		t.Fatalf("empty state = %q", got)
	}
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	monitor := &Monitor{status: Status{
		Available: true, LastCheck: now,
		Missing: []Permission{{Resource: "pods"}},
	}}
	snapshot := monitor.Snapshot()
	snapshot.Missing[0].Resource = "changed"
	if monitor.Snapshot().Missing[0].Resource != "pods" {
		t.Fatal("snapshot shares missing permissions")
	}
	if monitor.SecurityStatus().State != "healthy" {
		t.Fatal("healthy status did not set state")
	}
	if body, err := monitor.StatusJSON(); err != nil || len(body) == 0 {
		t.Fatalf("status JSON = %q, %v", body, err)
	}
}

func TestConfigureSourcesIsOneTimeAndCopiesNamespaces(t *testing.T) {
	monitor := &Monitor{infrastructure: []Permission{{Resource: "configmaps"}}}
	namespaces := []string{"apps"}
	if err := monitor.ConfigureSources(Sources{
		Namespaces: namespaces, InfrastructureNamespace: "kwatch",
	}); err != nil {
		t.Fatal(err)
	}
	namespaces[0] = "changed"
	if monitor.namespaces[0] != "apps" ||
		monitor.infrastructure[0].Namespace != "kwatch" {
		t.Fatal("source configuration was not copied")
	}
	if err := monitor.ConfigureSources(Sources{}); err == nil {
		t.Fatal("second source configuration succeeded")
	}
	monitor.started = true
	if err := monitor.ConfigureSources(Sources{}); err == nil {
		t.Fatal("configuration after start succeeded")
	}
}
