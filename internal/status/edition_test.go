package status

import (
	"strings"
	"testing"
	"time"
)

func TestCommunityTextNamesTheEditionAndHidesTheSelfCheck(t *testing.T) {
	r := sampleReport()
	r.Edition = "Community"
	r.ProInactive = []string{"delivery.audit-log", "policy.runbooks"}
	r.Brief = true
	r.CauseChecks = []CauseCheck{{Rule: "r", Text: "confirmed"}}
	r.Unexplained = Unexplained{Events: 3}

	text := r.Text()

	for _, want := range []string{
		"\nEdition: Community\n",
		"Pro features configured but inactive: delivery.audit-log, " +
			"policy.runbooks\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
	for _, hidden := range []string{"self-check", "Unexplained"} {
		if strings.Contains(text, hidden) {
			t.Fatalf("%q must be hidden in\n%s", hidden, text)
		}
	}
}

func TestCommunityTextWithNothingInactiveSaysOnlyTheEdition(t *testing.T) {
	r := sampleReport()
	r.Edition, r.Brief = "Community", true
	text := r.Text()
	if !strings.Contains(text, "Edition: Community") ||
		strings.Contains(text, "configured but inactive") {
		t.Fatalf("unexpected text\n%s", text)
	}
}

func TestProTextKeepsTheSelfCheckAndUnexplained(t *testing.T) {
	r := sampleReport()
	r.Edition = "Pro"
	r.CauseChecks = []CauseCheck{{Rule: "r", Text: "confirmed"}}
	text := r.Text()
	for _, want := range []string{"Cause self-check", "Unexplained",
		"Edition: Pro"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
}

func TestReportWithoutAnEditionHasNoEditionSection(t *testing.T) {
	if strings.Contains(sampleReport().Text(), "Edition") {
		t.Fatal("an unnamed edition must not print a section")
	}
}

func TestLicenseSectionShowsEverythingButTheKey(t *testing.T) {
	at := time.Date(2026, 10, 1, 8, 30, 0, 0, time.UTC)
	days := 12
	r := sampleReport()
	r.License = &License{
		Edition: "Pro", State: "grace", Plan: "pro", LID: "lic_…abcd",
		DaysLeft: &days, GraceUntil: &at, LastRenew: &at,
		NextAttempt: &at, Reason: "lease expired; renew soon",
	}

	text := r.Text()

	for _, want := range []string{
		"\nLicense\n", "  Edition: Pro\n", "  State: grace\n",
		"  Plan: pro\n", "  License ID: lic_…abcd\n", "  Days left: 12\n",
		"  Grace until: 2026-10-01 08:30 UTC\n",
		"  Last renewed: 2026-10-01 08:30 UTC\n",
		"  Next attempt: 2026-10-01 08:30 UTC\n",
		"  Reason: lease expired; renew soon\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
}

func TestLicenseSectionHintsAtTheTransferOnConflict(t *testing.T) {
	r := sampleReport()
	r.License = &License{
		Edition: "Community", State: "conflict", Hint: ConflictHint,
	}
	text := r.Text()
	want := "license is bound to another cluster — transfer it at " +
		"https://kwatch.dev/account"
	if !strings.Contains(text, want) {
		t.Fatalf("missing the hint in\n%s", text)
	}
}

func TestReportWithoutALicenseHasNoLicenseSection(t *testing.T) {
	if strings.Contains(sampleReport().Text(), "License") {
		t.Fatal("no license information, no section")
	}
}
