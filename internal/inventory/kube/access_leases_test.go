package kube

import "testing"

func TestSourceAccessLimitsKubeSystemLeasesToLeaders(t *testing.T) {
	names := map[string]bool{}
	for _, access := range SourceAccess() {
		if access.Resource.Name != "leases" ||
			access.Namespace != "kube-system" {
			continue
		}
		if access.Name == "" {
			t.Fatalf("kube-system lease access %+v is not named", access)
		}
		names[access.Name] = true
	}
	for _, want := range []string{Scheduler.Name, ControllerManager.Name} {
		if !names[want] {
			t.Errorf("missing named lease access for %s", want)
		}
	}
}
