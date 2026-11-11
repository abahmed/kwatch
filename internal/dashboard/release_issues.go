package dashboard

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/redact"
	"github.com/abahmed/kwatch/internal/release"
)

// releaseMatchWindow is how long after a release an issue on its
// workload counts as one that release may have caused: the window
// the release watch itself judges in (release.incidentVerdict).
const releaseMatchWindow = release.StallAfter + release.WatchWindow

// linkReleaseIssues names, for each release that went wrong, the issue
// it opened, resolved or not: the one the release watch linked, else
// the first issue on the same workload that opened from a minute
// before it to releaseMatchWindow after. The release watch links only
// issues people were told of, so an issue that settled on its own is
// found here. A release with neither keeps Issue nil.
func linkReleaseIssues(
	sum *ReleaseSummary, items []incident.Incident,
	red func(string) string,
) {
	if sum == nil {
		return
	}
	if red == nil {
		red = redact.Evidence
	}
	sum.WindowHours = int(releaseWindow / time.Hour)
	sum.MatchMinutes = int(releaseMatchWindow / time.Minute)
	for i := range sum.Wrong {
		w := &sum.Wrong[i]
		if p, ok := releaseIssue(*w, items); ok {
			item := toListItemWith(p, red)
			w.Issue = &SimilarRef{ID: item.ID, Title: item.Title,
				State: item.State}
		}
	}
}

// releaseIssue is the issue w is linked to, or the first one on its
// workload that opened within releaseMatchWindow after it.
func releaseIssue(
	w WrongRelease, items []incident.Incident,
) (incident.Incident, bool) {
	var best incident.Incident
	found := false
	from := w.At.Add(-releaseSlack)
	to := w.At.Add(releaseMatchWindow)
	for _, p := range items {
		if w.Incident != "" {
			if p.ID == w.Incident {
				return p, true
			}
			continue
		}
		if p.Tier == incident.Silent || !onWorkload(p, w) ||
			p.Opened.Before(from) || p.Opened.After(to) {
			continue
		}
		if !found || p.Opened.Before(best.Opened) {
			best, found = p, true
		}
	}
	return best, found
}

// onWorkload reports an issue whose root is w's workload.
func onWorkload(p incident.Incident, w WrongRelease) bool {
	return strings.EqualFold(string(p.Root.Kind), w.Kind) &&
		p.Root.Namespace == w.Namespace && p.Root.Name == w.Name
}
