package dashboard

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/redact"
)

// provenCause is the cause a detector proved from what a member
// reported, said before any timing: an image not built for its node's
// CPU, as the "exec format error" on a node of a known architecture
// shows. "" when no member proves one. The first member by name is
// used, so the sentence is stable.
func provenCause(p incident.Incident) string {
	best, found := detection.Finding{}, false
	for _, f := range p.Members {
		if f.Advisory || evidenceOf(f, detection.EvidenceArchNode) == "" {
			continue
		}
		if !found || f.Entity.String() < best.Entity.String() {
			best, found = f, true
		}
	}
	if !found {
		return ""
	}
	image := "Its image"
	if ref := evidenceOf(best, detection.EvidenceArchImage); ref != "" {
		image += " " + shortImage(ref)
	}
	return redact.Evidence(image + " is not built for the CPU of " +
		evidenceOf(best, detection.EvidenceArchNode) +
		", so it fails with \"exec format error\".")
}

// evidenceOf is the finding's first value for label, "" without one.
func evidenceOf(f detection.Finding, label string) string {
	for _, e := range f.Evidence {
		if e.Label == label {
			return strings.TrimSpace(e.Value)
		}
	}
	return ""
}
