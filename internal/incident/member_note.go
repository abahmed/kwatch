package incident

import (
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
