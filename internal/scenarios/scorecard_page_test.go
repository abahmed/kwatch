package scenarios

import (
	"os"
	"strings"
	"testing"
)

// SCORECARD.md commits the generated report between these markers, so the
// gate run can read it back and refuse a stale number.
const (
	docBegin = "<!-- BEGIN GENERATED REPORT (make alert-quality) -->"
	docEnd   = "<!-- END GENERATED REPORT (make alert-quality) -->"
)

// checkScorecardPage fails when the report block committed in SCORECARD.md
// has drifted from the report this run just produced. Run `make
// alert-quality` and paste _output/alert-quality.md between the markers to
// refresh it.
func checkScorecardPage(t *testing.T, report string) {
	t.Helper()
	page, err := os.ReadFile("SCORECARD.md")
	if err != nil {
		t.Fatalf("read SCORECARD.md: %v", err)
	}
	block, ok := reportBlock(string(page))
	if !ok {
		t.Fatal("SCORECARD.md is missing the generated report markers")
	}
	if strings.TrimSpace(block) != strings.TrimSpace(report) {
		t.Error("SCORECARD.md current report is stale: run " +
			"`make alert-quality` and replace the block between the " +
			"markers with _output/alert-quality.md")
	}
}

// reportBlock returns the text between the report markers in the page.
func reportBlock(page string) (string, bool) {
	start := strings.Index(page, docBegin)
	if start < 0 {
		return "", false
	}
	rest := page[start+len(docBegin):]
	end := strings.Index(rest, docEnd)
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}
