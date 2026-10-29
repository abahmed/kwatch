package dashboard

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/redact"
)

// toEvidence converts the report, redacting every string again: it was
// redacted at capture, this keeps the API safe on its own.
func toEvidence(report []incident.Section) []EvidenceSection {
	if len(report) == 0 {
		return nil
	}
	out := make([]EvidenceSection, 0, len(report))
	for _, s := range report {
		out = append(out, EvidenceSection{
			Check: s.Check, Title: s.Title, Status: s.Status,
			Reason:  s.Reason,
			Subject: redact.Evidence(s.Subject), Run: s.Run,
			Facts: toFacts(s.Facts), Lines: redactAll(s.Lines),
			Matched: append([]int{}, s.Matched...),
		})
	}
	return out
}

func toCorrelation(c *incident.Correlation) *Correlation {
	if c == nil {
		return nil
	}
	return &Correlation{Confirmed: redact.Evidence(c.Confirmed),
		Facts: toFacts(c.Facts)}
}

func toFacts(facts []incident.SectionFact) []EvidenceFact {
	out := make([]EvidenceFact, 0, len(facts))
	for _, f := range facts {
		out = append(out, EvidenceFact{Label: redact.Evidence(f.Label),
			Value: redact.Evidence(f.Value)})
	}
	return out
}
