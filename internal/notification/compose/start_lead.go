package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// startLead words a release whose new pods cannot start while the
// previous version still serves (incident.Incident.PreviousServes): not
// an outage, so neither "is down" nor a probe budget. "api in shop: the
// new version (api:2.3) keeps failing; the previous version is still
// serving", and when its containers stopped with a fatal line at start
// (explain.FatalStartLine, kept on the record) "… can't start; …: it
// stops at start with "…"", or, for a binary built for another CPU,
// ": it is not built for this node's CPU (arm64): "…"". A cause in
// another object (a missing Secret) keeps its own lead.
func startLead(f caseFacts) (string, bool) {
	if !f.p.PreviousServes || blamesAnother(f) {
		return "", false
	}
	name := f.leadName(affectedWorkload(f)) + ": the new version" +
		releaseLabel(f)
	const serving = "; the previous version is still serving"
	if f.p.Cause != nil && explain.FatalStartLine(f.p.Cause.SharedError) {
		return name + " can't start" + serving + ": it " +
			startErrorWords(f), true
	}
	return name + " keeps failing" + serving, true
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
	if strings.Contains(strings.ToLower(line), "exec format error") {
		cpu := "this node's CPU"
		if arch := archOf(f); arch != "" {
			cpu += " (" + arch + ")"
		}
		return "is not built for " + cpu + ": " + quoted(line)
	}
	return "stops at start with " + quoted(line)
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
