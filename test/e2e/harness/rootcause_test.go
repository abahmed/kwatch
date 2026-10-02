package harness

import (
	"testing"
	"time"
)

func TestEvaluateRootAcceptsExpectedRootTierAndBudget(t *testing.T) {
	entries := []AuditEntry{
		{Incident: "i1", Action: "create", Namespace: "ns",
			Root: "Node//n1", Tier: "page"},
		{Incident: "i1", Action: "update", Namespace: "ns",
			Root: "Node//n1", Tier: "page"},
		{Incident: "i1", Action: "resolved", Namespace: "ns",
			Root: "Node//n1"},
	}
	exp := RootExpectation{
		Root: "node//n1", Tier: "page", MaxMessages: 2,
		MustNotBlame: []string{"pod/ns/*"},
	}
	verdict := EvaluateRoot(entries, exp, RootScope{Namespace: "ns"})
	if err := verdict.Err(); err != nil {
		t.Fatal(err)
	}
	if verdict.Messages != 2 {
		t.Fatalf("messages = %d, want 2", verdict.Messages)
	}
}

func TestEvaluateRootReportsEveryViolation(t *testing.T) {
	entries := []AuditEntry{
		{Incident: "i1", Action: "create", Root: "Deployment/ns/api",
			Tier: "digest"},
		{Incident: "i1", Action: "update", Root: "Deployment/ns/api"},
		{Incident: "i1", Action: "update", Root: "Deployment/ns/api"},
		{Incident: "i2", Action: "create", Root: "Node//n1", Tier: "page"},
	}
	exp := RootExpectation{
		Root: "deployment/ns/api", Tier: "notify", MaxMessages: 2,
		MustNotBlame: []string{"node//n1"},
	}
	verdict := EvaluateRoot(entries, exp, RootScope{})
	if len(verdict.Problems) != 3 {
		t.Fatalf("problems = %v, want tier, budget and blame",
			verdict.Problems)
	}
}

func TestEvaluateRootMissingRootIsNotRooted(t *testing.T) {
	verdict := EvaluateRoot(nil, RootExpectation{Root: "pod/ns/a"},
		RootScope{})
	if verdict.Rooted || verdict.Err() == nil {
		t.Fatalf("empty audit must not satisfy a root: %+v", verdict)
	}
}

func TestEvaluateRootIgnoresEntriesOutsideScope(t *testing.T) {
	old := time.Unix(100, 0)
	entries := []AuditEntry{
		{Incident: "i1", Action: "create", Timestamp: old,
			Root: "Node//n1", Tier: "page"},
		{Incident: "i2", Action: "create", Namespace: "other",
			Root: "Pod/other/x"},
	}
	exp := RootExpectation{Root: "node//n1", MustNotBlame: []string{
		"pod/other/x"}}
	scope := RootScope{Namespace: "ns", Since: time.Unix(200, 0)}
	if EvaluateRoot(entries, exp, scope).Rooted {
		t.Fatal("entry before Since must be out of scope")
	}
	scope = RootScope{Namespace: "ns", Since: time.Unix(50, 0)}
	if v := EvaluateRoot(entries, exp, scope); v.Err() != nil {
		t.Fatalf("other namespace must be ignored: %v", v.Err())
	}
}

func TestRootMatchesGroupAwareAndPrefix(t *testing.T) {
	tests := []struct {
		actual, want string
		ok           bool
	}{
		{"Registry//reg.example", "registry//reg.example", true},
		{"Pod/ns/api-7d9-x", "pod/ns/api-*", true},
		{"Pod/ns/web-1", "pod/ns/api-*", false},
		{"Pod/ns/api", "pod/other/api", false},
		{"bad", "pod/ns/api", false},
	}
	for _, tt := range tests {
		if got := rootMatches(tt.actual, tt.want); got != tt.ok {
			t.Errorf("rootMatches(%q,%q) = %v", tt.actual, tt.want, got)
		}
	}
}

func TestEvaluateRootBoundsTotalMessagesAcrossIncidents(t *testing.T) {
	var entries []AuditEntry
	for _, id := range []string{"a", "b", "c", "d"} {
		entries = append(entries, AuditEntry{
			Incident: id, Action: "create", Root: "Pod/ns/" + id,
		})
	}
	entries[0].Root = "Deployment/ns/storm"
	exp := RootExpectation{Root: "deployment/ns/storm", MaxTotalMessages: 3}
	if EvaluateRoot(entries, exp, RootScope{}).Err() == nil {
		t.Fatal("four messages must exceed a total budget of three")
	}
	exp.MaxTotalMessages = 4
	if err := EvaluateRoot(entries, exp, RootScope{}).Err(); err != nil {
		t.Fatal(err)
	}
}
