package incident

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Records written before causes carried their mode and proof codes keep
// both only in explain's words: the mode inside the summary ("secret db
// (Missing) explains 2 failures") and each proof as a sentence. The
// patterns below are those earlier words, frozen: restoring such a
// record derives the structured fields from them once, so writers never
// read text. Explain's current wording does not matter here.
var legacyProofs = []struct {
	pattern *regexp.Regexp
	apply   func(p *rootcause.Proof, m []string)
}{
	{regexp.MustCompile(`^(\d+) of (\d+) dependents fail$`),
		legacyCounted(rootcause.ProofDependentsFail)},
	{regexp.MustCompile(`^the errors of (\d+) of (\d+) failures name it$`),
		legacyCounted(rootcause.ProofErrorsName)},
	{regexp.MustCompile(`^failures of (\d+) workloads meet here$`),
		legacyCounted(rootcause.ProofWorkloadsMeet)},
	{regexp.MustCompile(`^it began before the failures$`),
		legacyCounted(rootcause.ProofBeganBefore)},
	{regexp.MustCompile(`^nothing it depends on explains its failures$`),
		legacyCounted(rootcause.ProofNothingUpstream)},
	{regexp.MustCompile(`^it changed shortly before: (.+)$`),
		legacyChange},
}

// legacyCounted sets code and the numbers the pattern captured, in
// order: Count, then Total.
func legacyCounted(
	code rootcause.ProofCode,
) func(*rootcause.Proof, []string) {
	return func(p *rootcause.Proof, m []string) {
		p.Code = code
		if len(m) > 1 {
			p.Count, _ = strconv.Atoi(m[1])
		}
		if len(m) > 2 {
			p.Total, _ = strconv.Atoi(m[2])
		}
	}
}

// legacyChange reads what an earlier change proof named: "created",
// "changed" or the field paths it touched.
func legacyChange(p *rootcause.Proof, m []string) {
	switch m[1] {
	case "created":
		p.Code = rootcause.ProofCreated
	case "changed":
		p.Code = rootcause.ProofChanged
	default:
		p.Code = rootcause.ProofChanged
		p.Fields = strings.Split(m[1], ", ")
	}
}

// restoredCause returns a detached copy of a persisted cause with the
// mode and proof codes of an earlier record filled in from its words.
// Records that already carry them are copied unchanged.
func restoredCause(c *rootcause.CauseRecord) *rootcause.CauseRecord {
	if c == nil {
		return nil
	}
	out := *c
	if out.Mode == "" {
		out.Mode = legacySummaryMode(out.Summary)
	}
	out.Proof = append([]rootcause.Proof(nil), c.Proof...)
	for i := range out.Proof {
		if out.Proof[i].Code == "" {
			applyLegacyProof(&out.Proof[i])
		}
	}
	return &out
}

// legacySummaryMode is the mode an earlier summary held in parentheses.
func legacySummaryMode(summary string) detection.Mode {
	_, rest, found := strings.Cut(summary, " (")
	if !found {
		return ""
	}
	mode, _, _ := strings.Cut(rest, ")")
	return detection.Mode(mode)
}

// applyLegacyProof fills the code of an earlier proof from its words.
// Words no pattern knows stay text, as they were written.
func applyLegacyProof(p *rootcause.Proof) {
	for _, legacy := range legacyProofs {
		if m := legacy.pattern.FindStringSubmatch(p.Text); m != nil {
			legacy.apply(p, m)
			return
		}
	}
}
