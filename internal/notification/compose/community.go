package compose

import (
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// This file is the Community edition of a message. The engine still
// finds the cause, the changes and the impact; a Community message says
// what broke and the one-line cause, then what to do, and drops the
// detail: proofs, counterfactual checks, confidence, the recent change,
// affected services and investigation output.

// ForEdition returns a writer for the edition e entitles. An edition
// without root cause detail writes the Community message; its Pro note
// follows e.Limits().ProHints unless hideHints is set. A nil e is the
// Community edition.
func (w Writer) ForEdition(e feature.Entitlements, hideHints bool) Writer {
	e = feature.OrCommunity(e)
	if e.Limits().RootCauseDetail {
		return w
	}
	w.Community = true
	w.ProNote = e.Limits().ProHints && !hideHints
	return w
}

// communityParts are the parts of a note that carry detail Pro adds.
var communityParts = map[part]bool{
	partProof: true, partChanges: true, partConsequence: true,
	partChecked: true,
}

// communityDecision is d without the detail a Community message omits.
// The decision is copied: the incident keeps its cause, and /status and
// the audit log read it whole.
func communityDecision(d incident.Decision) incident.Decision {
	d.Facts.Output, d.Facts.Evidence = nil, nil
	d.Facts.Report, d.Facts.Correlation = nil, nil
	d.Facts.Changes, d.Facts.Window, d.Facts.Wake = nil, nil, nil
	d.Incident.Cause = communityCause(d.Incident.Cause)
	return d
}

// communityCause keeps the cause's identity and wording and drops its
// proof, checks, blamed change and failure chain.
func communityCause(c *rootcause.CauseRecord) *rootcause.CauseRecord {
	if c == nil {
		return nil
	}
	plain := *c
	plain.Proof, plain.Checked, plain.Change = nil, nil, nil
	plain.RollbackRevision = ""
	plain.Hops, plain.Beyond = nil, nil
	plain.Rival = communityCause(c.Rival)
	return &plain
}

// communitySentences drops the sentences that carry Pro detail and, for
// a first message or an update, adds the Pro note.
func (w Writer) communitySentences(
	sentences []sentence, note string,
) []sentence {
	var out []sentence
	for _, s := range sentences {
		if !communityParts[s.part] {
			out = append(out, s)
		}
	}
	if note != "" && len(out) > 0 {
		out = append(out, sentence{part: partHint, text: note})
	}
	return out
}
