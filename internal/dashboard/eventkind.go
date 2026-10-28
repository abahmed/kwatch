package dashboard

import (
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
)

// Timeline event kinds. They are the API contract: the UI words and
// colours an event by its kind, never by parsing the English text, which
// the incident manager may reword.
const (
	kindRepeated     = "repeated"
	kindCauseRevised = "cause_revised"
	kindAcknowledged = "acknowledged"
	kindAckRemoved   = "ack_removed"
	kindFlapping     = "flapping"
	kindPagedAgain   = "paged_again"
	kindPagedWith    = "paged_together"
	kindSameOutage   = "same_outage"
	kindResolved     = "resolved"
	kindNote         = "note"
)

// eventKinds maps the start of a timeline note (internal/incident note
// calls) to its kind. Order matters only where prefixes overlap.
var eventKinds = []struct{ prefix, kind string }{
	{"failing again: ", kindRepeated},
	{"happened again (", kindRepeated},
	{"acknowledged on ", kindAcknowledged},
	{"acknowledgement removed", kindAckRemoved},
	{"cause revised", kindCauseRevised},
	{"flapping: ", kindFlapping},
	{"still failing: paging again", kindPagedAgain},
	{"announced with a page for the same moment", kindPagedWith},
	{"same outage as ", kindSameOutage},
	{"resolved: ", kindResolved},
}

// Reasons of a resolved event.
const (
	reasonRecovered        = "recovered"
	reasonRecoveredUnheard = "recovered_unheard"
	reasonSuperseded       = "superseded"
	reasonStoppedTracking  = "stopped_tracking"
	reasonOther            = "other"
)

var resolveReasons = []struct{ prefix, reason string }{
	{"healthy for ", reasonRecovered},
	{"stable for ", reasonRecovered},
	{string(incident.ReasonRecoveredUnheard), reasonRecoveredUnheard},
	{"reopened and recovered before it was reported",
		reasonRecoveredUnheard},
	{string(incident.ReasonSuperseded), reasonSuperseded},
	{incident.StoppedTrackingPrefix, reasonStoppedTracking},
}

// classifyEvent returns the kind of a timeline note and, for a resolve,
// its reason. Unknown text is a plain note, never dropped.
func classifyEvent(text string) (kind, reason string) {
	for _, k := range eventKinds {
		rest, ok := strings.CutPrefix(text, k.prefix)
		if !ok {
			continue
		}
		if k.kind == kindResolved {
			return kindResolved, resolveReason(rest)
		}
		return k.kind, ""
	}
	return kindNote, ""
}

func resolveReason(rest string) string {
	for _, r := range resolveReasons {
		if strings.HasPrefix(rest, r.prefix) {
			return r.reason
		}
	}
	return reasonOther
}
