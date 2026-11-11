//go:build e2e

package harness

import (
	"strings"
	"testing"
)

func TestMatchingEntriesAcceptsNamespacedResourceNames(t *testing.T) {
	entries := []AuditEntry{{
		Namespace: "apps",
		Root:      "Deployment/apps/api",
		Reason:    "ReplicaFailure,DeploymentUnavailable",
		Action:    "create",
	}}

	tests := []struct {
		name  string
		match AuditMatch
		want  int
	}{
		{
			name: "short name with namespace",
			match: AuditMatch{
				Namespace: "apps", Resource: "api",
				Reason: "DeploymentUnavailable", Count: 1,
			},
			want: 1,
		},
		{
			name: "fully qualified name",
			match: AuditMatch{
				Namespace: "apps", Resource: "apps/api",
				Reason: "DeploymentUnavailable", Count: 1,
			},
			want: 1,
		},
		{
			name: "one reason of a joined list",
			match: AuditMatch{
				Namespace: "apps", Resource: "api",
				Reason: "DeploymentUnavailable", Count: 1,
			},
			want: 1,
		},
		{
			name: "wrong namespace",
			match: AuditMatch{
				Namespace: "other", Resource: "api",
				Reason: "DeploymentUnavailable", Count: 1,
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(matchingEntries(entries, tt.match)); got != tt.want {
				t.Fatalf("matching entries = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMatchingEntriesAcceptsContainerRoots(t *testing.T) {
	entries := []AuditEntry{{
		Namespace: "apps",
		Root:      "container/apps/post-start/workload",
		Reason:    "FailedPostStartHook",
		Action:    "update",
	}}
	for resource, want := range map[string]int{
		"post-start": 1, "workload": 0,
	} {
		match := AuditMatch{Namespace: "apps", Resource: resource, Count: 1}
		if got := len(matchingEntries(entries, match)); got != want {
			t.Fatalf("resource %q matched %d, want %d", resource, got, want)
		}
	}
}

func TestParseAuditReadsLinesLongerThanDefaultScannerBuffer(t *testing.T) {
	long := `{"action":"create","note":"` + strings.Repeat("x", 100<<10) + `"}`
	payload := []byte(long + "\n" + `{"action":"resolve"}` + "\n")

	entries, err := parseAudit(payload)

	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[1].Action != "resolve" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParseAuditReportsOversizedLine(t *testing.T) {
	huge := strings.Repeat("x", maxAuditLineBytes+1)

	_, err := parseAudit([]byte(huge + "\n"))

	if err == nil {
		t.Fatal("an oversized line must be reported, not dropped silently")
	}
}

// toldAgainRun is the audit of one CI run of the re-failure scenario:
// the first incident, its resolve, and how the return was told.
func toldAgainRun(root string, back AuditEntry) []AuditEntry {
	reason := "CreateContainerConfigError,ProjectedConfigMapMissing"
	return []AuditEntry{
		{Action: "create", Incident: "inc-1", Namespace: "shop",
			Root: "configmap/shop/settings", Reason: reason},
		{Action: "resolved", Incident: "inc-1", Namespace: "shop",
			Root: root, Reason: reason},
		back,
	}
}

// Both outcomes seen on CI count as telling the failure again: a reopen
// with "failing again" when the incident kept the ConfigMap root, a new
// incident when its root had moved to the Deployment while recovering.
func TestToldAgainAcceptsAReopenOrANewIncident(t *testing.T) {
	reason := "CreateContainerConfigError,ProjectedConfigMapMissing"
	reopen := toldAgainRun("configmap/shop/settings", AuditEntry{
		Action: "update", Incident: "inc-1", Namespace: "shop",
		Root: "configmap/shop/settings", Reason: reason,
		DecisionReason: FailingAgain,
	})
	recur := toldAgainRun("deployment/shop/web", AuditEntry{
		Action: "create", Incident: "inc-2", Namespace: "shop",
		Root: "configmap/shop/settings", Reason: reason,
	})
	quiet := toldAgainRun("configmap/shop/settings", AuditEntry{
		Action: "update", Incident: "inc-1", Namespace: "shop",
		Root: "configmap/shop/settings", Reason: reason,
		DecisionReason: "fix attempt",
	})
	matches := ToldAgain(
		"shop", "inc-1", "settings", "ProjectedConfigMapMissing")
	for name, tc := range map[string]struct {
		entries []AuditEntry
		want    bool
	}{
		"reopened": {reopen, true}, "new incident": {recur, true},
		"not told again":    {quiet, false},
		"before the return": {reopen[:2], false},
	} {
		if _, got := firstMet(tc.entries, matches); got != tc.want {
			t.Errorf("%s: told again = %v, want %v", name, got, tc.want)
		}
	}
}
