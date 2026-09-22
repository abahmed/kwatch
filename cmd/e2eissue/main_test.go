package main

import "testing"

func TestParseIssueURL(t *testing.T) {
	owner, repository, number, err := parseIssueURL(
		"https://github.com/abahmed/kwatch/issues/123",
	)
	if err != nil {
		t.Fatal(err)
	}
	if owner != "abahmed" || repository != "kwatch" || number != 123 {
		t.Fatalf("unexpected issue identity: %s/%s/%d", owner, repository, number)
	}
}

func TestParseIssueURLRejectsNonGitHubURL(t *testing.T) {
	if _, _, _, err := parseIssueURL(
		"https://example.com/issues/123",
	); err == nil {
		t.Fatal("expected non-GitHub URL to be rejected")
	}
}
