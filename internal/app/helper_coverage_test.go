package app

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
)

func TestActiveGraphKeyCheckerRefreshesAfterTTL(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	reads := 0
	checker := newActiveGraphKeyChecker(
		func() map[model.IncidentKey]*model.Incident {
			reads++
			return map[model.IncidentKey]*model.Incident{
				"incident": {Subject: model.Subject{
					Object: model.ObjectRef{
						Kind: "pod", Namespace: "apps", Name: "api",
					},
				}},
			}
		},
		func() time.Time { return now },
	)
	if !checker("pod", "apps", "api") || checker("pod", "apps", "other") {
		t.Fatal("active graph key lookup returned the wrong result")
	}
	if reads != 1 {
		t.Fatalf("active incidents read %d times before expiry", reads)
	}
	now = now.Add(activeKeyTTL + time.Nanosecond)
	if !checker("pod", "apps", "api") || reads != 2 {
		t.Fatalf("active graph key cache did not refresh: reads=%d", reads)
	}
}

func TestAppHelperBranches(t *testing.T) {
	if got := describeDependency("not-a-key"); got != "not-a-key" {
		t.Fatalf("malformed dependency = %q", got)
	}
	if got := describeDependency("node//worker-1"); got != "node worker-1" {
		t.Fatalf("dependency description = %q", got)
	}
	inc := &model.Incident{Subject: model.Subject{
		Object: model.ObjectRef{Kind: "node", Name: "worker-1"},
	}}
	if !coveredByNodeIncident("node//worker-1", []*model.Incident{inc}) {
		t.Fatal("node incident was not recognized")
	}
	if coveredByNodeIncident("service/apps/api", []*model.Incident{inc}) {
		t.Fatal("non-node dependency was incorrectly covered")
	}
	for _, reason := range []string{
		"cache_sync_failed", "source_not_configured",
		"optional_api_unavailable", "watcher_failed",
	} {
		if got := crdWatcherReason(reason); got != reason {
			t.Fatalf("crdWatcherReason(%q) = %q", reason, got)
		}
	}
	if got := crdWatcherReason("unexpected"); got != "watcher_failed" {
		t.Fatalf("unknown watcher reason = %q", got)
	}
	if !activeProbesEnabled(config.ActiveProbeMonitor{AutoServices: true}) {
		t.Fatal("auto services did not enable probes")
	}
	if activeProbesEnabled(config.ActiveProbeMonitor{}) {
		t.Fatal("empty probe configuration enabled probes")
	}
}
