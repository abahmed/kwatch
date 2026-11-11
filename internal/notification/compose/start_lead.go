package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// startLead words a release whose new pods cannot start while the
// previous version still serves (incident.Incident.PreviousServes): not
// an outage, so neither "is down" nor a probe budget. "api in shop: the
// new version (api:2.3) keeps failing; the previous version (api:2.2) is
// still serving", and when its containers stopped with a fatal line at
// start (explain.FatalStartLine, kept on the record) "… can't start (it
// stops at start with "…"); …", or, for a binary built for another CPU,
// "(it is not built for this node's CPU (arm64): "…")". A cause in
// another object (a missing Secret) keeps its own lead.
func startLead(f caseFacts) (string, bool) {
	if !f.p.PreviousServes || blamesAnother(f) {
		return "", false
	}
	name := f.leadName(affectedWorkload(f)) + ": the new version" +
		releaseLabel(f)
	serving := "; the previous version" + previousLabel(f) +
		" is still serving"
	if f.p.Cause != nil && explain.FatalStartLine(f.p.Cause.SharedError) {
		return name + " can't start (it " + startErrorWords(f) + ")" +
			serving, true
	}
	return name + " keeps failing" + serving, true
}

// previousLabel is " (api:2.2)", the image the blamed release replaced,
// or nothing. The start lead says it, so no other sentence repeats it
// (see previousSaidByLead).
func previousLabel(f caseFacts) string {
	change := blamedChange(f)
	if change == nil {
		return ""
	}
	before := previousImage(*change)
	if i := strings.LastIndex(before, "/"); i >= 0 {
		before = before[i+1:]
	}
	if before == "" {
		return ""
	}
	return " (" + before + ")"
}

// previousImage is the image a release of one image replaced, or "".
func previousImage(change inventory.Change) string {
	if len(change.Fields) != 1 {
		return ""
	}
	field := change.Fields[0]
	if releasedImage(change) != field.After {
		return ""
	}
	return field.Before
}

// previousSaidByLead reports a start lead that names the previous
// version and says that it still serves: the change sentence and the
// proof that only the new revision fails would repeat it.
func previousSaidByLead(f caseFacts) bool {
	_, ok := startLead(f)
	return ok && previousLabel(f) != ""
}

// releaseLabel is " (api:2.3)" for a blamed release of an image, or
// nothing: the image's name and tag, without its registry path.
func releaseLabel(f caseFacts) string {
	change := blamedChange(f)
	if change == nil {
		return ""
	}
	image := releasedImage(*change)
	if image == "" {
		return ""
	}
	if i := strings.LastIndex(image, "/"); i >= 0 {
		image = image[i+1:]
	}
	return " (" + image + ")"
}

// startErrorWords is what the program does at start, as a predicate:
// "stops with "…"", or, for a binary built for another CPU, "is not
// built for this node's CPU (arm64): "exec …: exec format error"".
func startErrorWords(f caseFacts) string {
	line := f.p.Cause.SharedError
	if cpu := startCPUWords(f); cpu != "" {
		return cpu + ": " + quoted(line)
	}
	return "stops at start with " + quoted(line)
}

// startCPUWords are "is not built for this node's CPU (arm64)" for a
// binary built for another CPU, or "".
func startCPUWords(f caseFacts) string {
	line := f.p.Cause.SharedError
	if !strings.Contains(strings.ToLower(line), "exec format error") {
		return ""
	}
	cpu := "this node's CPU"
	if arch := archOf(f); arch != "" {
		cpu += " (" + arch + ")"
	}
	return "is not built for " + cpu
}

// startShortSwap is how the Short of a start lead drops the quoted
// start error, so a provider that cuts at 130 bytes keeps "the previous
// version is still serving": the parenthesis and what replaces it.
// The CPU a binary was not built for stays; the quotation goes.
func startShortSwap(f caseFacts) [2]string {
	text, ok := startLead(f)
	if !ok || f.p.Cause == nil {
		return [2]string{}
	}
	long := " (it " + startErrorWords(f) + ")"
	if !strings.Contains(text, long) {
		return [2]string{}
	}
	if cpu := startCPUWords(f); cpu != "" {
		return [2]string{long, " (it " + cpu + ")"}
	}
	return [2]string{long, ""}
}

// archOf is the node architecture the arch detector wrote ("arm64"),
// or "".
func archOf(f caseFacts) string {
	for _, m := range f.members {
		value := evidence(m, detection.EvidenceArchNode)
		if arch, _, ok := strings.Cut(value, " node "); ok {
			return arch
		}
	}
	return ""
}

// quotedByStartLead reports an error line the start lead quotes
// already: the message says it once.
func quotedByStartLead(f caseFacts, line string) bool {
	if _, ok := startLead(f); !ok || f.p.Cause == nil {
		return false
	}
	cause := strings.TrimSpace(f.p.Cause.SharedError)
	return cause != "" && strings.TrimSpace(line) == cause
}

// releasedByText is what the change sentence adds to a start lead that
// named both versions: who released it, or nothing.
func releasedByText(change inventory.Change) string {
	if change.Actor == "" {
		return ""
	}
	return change.Actor + " released it"
}

// restatedByStartLead reports a proof that only repeats a start lead
// which says the new version fails while the previous one serves.
func restatedByStartLead(f caseFacts, code rootcause.ProofCode) bool {
	return (code == rootcause.ProofNewRevisionFails ||
		code == rootcause.ProofFailedAfterChange) && previousSaidByLead(f)
}
