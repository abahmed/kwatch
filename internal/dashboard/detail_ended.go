package dashboard

import (
	"cmp"
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/redact"
)

// resolvedPrefix starts the timeline note of a resolve; the reason
// follows it.
const resolvedPrefix = "resolved: "

// resolveReasonOf is the reason of p's last resolve, from its
// timeline; "" while p is not resolved.
func resolveReasonOf(p incident.Incident) string {
	if p.State != incident.Resolved {
		return ""
	}
	for i := len(p.Timeline) - 1; i >= 0; i-- {
		rest, ok := strings.CutPrefix(p.Timeline[i].Text, resolvedPrefix)
		if ok {
			return rest
		}
	}
	return ""
}

// stoppedTracking reports a resolved p that kwatch stopped tracking at
// its still-broken cap rather than one that recovered.
func stoppedTracking(p incident.Incident) bool {
	return strings.HasPrefix(resolveReasonOf(p),
		incident.StoppedTrackingPrefix)
}

// stillCrashing reports a stopped-tracking resolve that held because a
// container kept crashing.
func stillCrashing(p incident.Incident) bool {
	reason := resolveReasonOf(p)
	return strings.HasPrefix(reason, incident.StoppedTrackingPrefix) &&
		strings.HasSuffix(reason, incident.StillCrashingNote)
}

// addEnd fills how p ended, what its pagers and channels hold, and
// whether its logs could not be read.
func (c detailContext) addEnd(d *IncidentDetail, p incident.Incident) {
	d.StillCrashing = stillCrashing(p)
	d.Moved = movedOf(p.Moved, c.lookup)
	if p.State == incident.Resolved {
		d.ChangedBeforeRecovery = changedBeforeRecovery(p)
	}
	d.LogsNotReadable = c.logsForbidden && len(d.CapturedLogLines) == 0
	d.Pager = toPager(c.delivery.Pager)
	d.Sent = toSent(c.delivery.Sent)
}

// changedBeforeRecovery is incident.ChangesBeforeRecovery of p, its
// fixing change left out, as the page lists changes.
func changedBeforeRecovery(p incident.Incident) []ChangeItem {
	changes := incident.ChangesBeforeRecovery(p, p.FixedBy)
	if len(changes) == 0 {
		return nil
	}
	onset := p.Onset()
	out := make([]ChangeItem, 0, len(changes))
	for _, change := range changes {
		out = append(out, toChangeItem(change, onset))
	}
	return out
}

// movedOf links the incidents of m that lookup still finds, with their
// titles; nil when none is left.
func movedOf(
	m incident.Moved, lookup func(string) *incident.Incident,
) *MovedLinks {
	if lookup == nil {
		return nil
	}
	ref := func(id string) *IncidentRef {
		if id == "" {
			return nil
		}
		q := lookup(id)
		if q == nil {
			return nil
		}
		return &IncidentRef{ID: id, Title: titleFor(*q, redact.Evidence)}
	}
	out := MovedLinks{Into: ref(m.Into)}
	for _, id := range m.From {
		if r := ref(id); r != nil {
			out.From = append(out.From, *r)
		}
	}
	if out.Into == nil && len(out.From) == 0 {
		return nil
	}
	return &out
}

// toPager names each pager and redacts its reason again.
func toPager(in []PagerRow) []PagerRow {
	if len(in) == 0 {
		return nil
	}
	out := make([]PagerRow, 0, len(in))
	for _, r := range in {
		r.Name = channelName(r.Provider)
		r.Reason = redact.Evidence(r.Reason)
		out = append(out, r)
	}
	return out
}

// toSent names each channel, redacts each text again and sorts them by
// provider.
func toSent(in []SentMessage) []SentMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]SentMessage, 0, len(in))
	for _, m := range in {
		m.Name = channelName(m.Provider)
		m.Text = redact.Evidence(m.Text)
		out = append(out, m)
	}
	slices.SortFunc(out, func(a, b SentMessage) int {
		return cmp.Compare(a.Provider, b.Provider)
	})
	return out
}
