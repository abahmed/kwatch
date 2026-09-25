//go:build e2e

package harness

import "testing"

func TestMatchingEntriesAcceptsNamespacedResourceNames(t *testing.T) {
	entries := []AuditEntry{{
		Namespace: "apps",
		Name:      "apps/api",
		Reason:    "DeploymentUnavailable",
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
