package incident

import (
	"slices"
	"strconv"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/redact"
)

// Evidence labels of a probe finding (detectors/container_probe.go):
// the kubelet's last message and how many failures it counted.
const (
	probeEvidence    = "probe"
	failuresEvidence = "failures"
	// maxNoteQuote bounds the probe message a timeline note quotes.
	maxNoteQuote = 160
)

// memberNote is the timeline text of a finding that joins an incident:
// its summary and, for a failing probe, how often it failed and the
// kubelet's last words, so a readiness check that keeps failing is on
// record with its count and message ("Container keeps failing its
// readiness probe: 23 failures, last: Readiness probe failed: …").
func memberNote(s detection.Finding) string {
	text := s.Summary
	message, count := "", 0
	for _, e := range s.Evidence {
		switch e.Label {
		case probeEvidence:
			message = e.Value
		case failuresEvidence:
			count, _ = strconv.Atoi(e.Value)
		}
	}
	if message == "" {
		return text + " (" + describe(s.Entity) + ")"
	}
	text += ": "
	if count > 1 {
		text += strconv.Itoa(count) + " failures, last: "
	}
	quote := truncateRunes(redact.Credentials(message), maxNoteQuote)
	if len(quote) < len(message) {
		quote += "…"
	}
	return text + quote + " (" + describe(s.Entity) + ")"
}

// rejoined reports finding s coming back to the restored incident p as
// a member it had when the record was saved: the same object with the
// same reason, as one pair. A restart rebuilds the members, and the
// timeline already has the note of its joining, so it is not written
// again. An object that failed for another reason, while some other
// member had this one, is a new member.
func rejoined(p *Incident, s detection.Finding) bool {
	return p.restored && slices.Contains(p.formerKeys, s.Key())
}
