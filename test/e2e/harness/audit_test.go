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
