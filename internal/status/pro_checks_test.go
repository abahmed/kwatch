package status

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/feature"
)

func TestCommunityTextCountsProChecks(t *testing.T) {
	r := sampleReport()
	r.Edition, r.Brief = "Community", true
	r.ProChecks = &feature.WithheldCounts{4, 2, 1, 0}
	want := "Pro checks: 7 issues in the last 7 days (4 readiness " +
		"probes, 2 shared causes, 1 preemption and admission). " +
		"Details are in Pro.\n"
	if text := r.Text(); !strings.Contains(text, want) {
		t.Fatalf("missing %q in\n%s", want, text)
	}
}

func TestCommunityTextSaysWhenProChecksFoundNothing(t *testing.T) {
	r := sampleReport()
	r.Edition, r.Brief = "Community", true
	r.ProChecks = &feature.WithheldCounts{}
	want := "Pro checks: no issues in the last 7 days.\n"
	if text := r.Text(); !strings.Contains(text, want) {
		t.Fatalf("missing %q in\n%s", want, text)
	}
}

func TestProTextHasNoProChecksLine(t *testing.T) {
	r := sampleReport()
	r.Edition = "Pro"
	if text := r.Text(); strings.Contains(text, "Pro checks") {
		t.Fatalf("pro text counts Pro checks:\n%s", text)
	}
}
