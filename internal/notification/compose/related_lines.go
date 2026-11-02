package compose

import (
	"net/url"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// Lines that tie an opening to what people already know: the incident
// that caused this one, and the earlier times the same root failed.
const (
	causeLinkLead   = "Cause in dashboard: "
	earlierLinkLead = "Earlier times: "
	// repeatOffender is how many times in a week, this one included,
	// make an incident a repeat offender: the recurrence line names the
	// object and the message links the earlier times.
	repeatOffender = 4
)

// causedBySentences name the open incident this one is caused by when
// people already heard about it: "Caused by: Node n1 is not ready."
func causedBySentences(f caseFacts) []sentence {
	cause, ok := f.p.CauseTold()
	if !ok {
		return nil
	}
	title := strings.TrimSpace(strings.TrimSuffix(cause.Title, "."))
	return []sentence{{part: partCause, text: "Caused by: " + title + "."}}
}

// weekTimes is how many times people heard of the incident's root
// failing in the week up to now, this time included.
func weekTimes(p incident.Incident, now time.Time) int {
	times := 1
	for _, o := range p.History {
		if o.Heard && now.Sub(o.Opened) <= format.Week {
			times++
		}
	}
	return times
}

// repeatSentences say, in a "failing again" update, how often the root
// failed this week when that is more than the update's own count:
// "This is the fourth time this week on deployment/api."
func repeatSentences(f caseFacts) []sentence {
	times := weekTimes(f.p, f.now)
	if times < repeatOffender || times <= f.p.RepeatCount {
		return nil
	}
	return []sentence{{part: partRecurrence, text: "This is the " +
		ordinalWord(times) + " time this week on " +
		objectName(f.p.Root) + "."}}
}

// opensStory reports a message that starts or restarts the story: an
// announcement, or an update that says the incident fails again.
func opensStory(d incident.Decision) bool {
	return d.Action == incident.Announce ||
		(d.Action == incident.Update &&
			d.Reason == incident.ReasonFailingAgain)
}

// withRelatedLinks adds the dashboard links of an opening: the page of
// the incident that caused it, and the list of the earlier times of a
// repeat offender. Nothing without a dashboard address.
func (w Writer) withRelatedLinks(
	d incident.Decision, now time.Time, msg notification.Message,
) notification.Message {
	base := w.dashboardBase()
	if base == "" || !opensStory(d) {
		return msg
	}
	if cause, ok := d.Incident.CauseTold(); ok {
		msg = withLink(msg, causeLinkLead+base+"/issues/"+
			url.PathEscape(cause.ID))
	}
	if weekTimes(d.Incident, now) >= repeatOffender {
		msg = withLink(msg, earlierLinkLead+base+
			"/issues?status=all&like="+url.QueryEscape(d.Incident.ID))
	}
	return msg
}
