package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/release"
)

// ReleaseCause is the release linked to an incident: what changed,
// how it went and how to undo it. The dashboard only shows it; the
// command is for the person to run.
type ReleaseCause struct {
	release.Record
	// RolloutSeconds is how long the rollout took, once it finished.
	RolloutSeconds *float64 `json:"rolloutSeconds,omitempty"`
	Undo           UndoStep `json:"undo"`
	// Newest says no later release of the workload exists.
	Newest bool `json:"newest"`
}

// UndoStep is how to undo a release: a text and, when one does it, the
// command.
type UndoStep struct {
	Text     string `json:"text"`
	Command  string `json:"command,omitempty"`
	Mutating bool   `json:"mutating,omitempty"`
}

// ReleaseSummary is the Overview's line about the last 24 hours.
type ReleaseSummary struct {
	Total int `json:"total"`
	Bad   int `json:"bad"`
	// BadIncidents are the issues the releases that went wrong
	// caused, for the link from "went wrong".
	BadIncidents []string `json:"badIncidents"`
	// Wrong lists the releases that went wrong, newest first, so one
	// that opened no issue (a regression) is still seen. Incident is
	// empty for it.
	Wrong []WrongRelease `json:"wrong"`
}

// maxWrong bounds Wrong.
const maxWrong = 5

// WrongRelease is a release that went wrong, in brief.
type WrongRelease struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Namespace string          `json:"namespace"`
	Name      string          `json:"name"`
	At        time.Time       `json:"at"`
	Verdict   release.Verdict `json:"verdict"`
	Incident  string          `json:"incident,omitempty"`
}

// releaseWindow is the window of the Overview's line.
const releaseWindow = 24 * time.Hour

// releaseBookSnapshot is a copy of the book, without its detail when
// the change history (Pro) is off: a book kept from a Pro run serves
// none of it.
func (s *Server) releaseBookSnapshot() (release.Book, bool) {
	book := s.sources.releases.Load()
	if book == nil {
		return release.Book{}, false
	}
	snap := book.snapshot()
	if !s.opts.ChangeHistory {
		stripDetail(&snap)
	}
	return snap, true
}

// releaseFor is the release that caused the incident, or nil.
func (s *Server) releaseFor(id string) *ReleaseCause {
	snap, ok := s.releaseBookSnapshot()
	if !ok {
		return nil
	}
	return releaseOf(snap, id)
}

// releaseOf finds the newest release of b linked to incident id.
func releaseOf(b release.Book, id string) *ReleaseCause {
	newest := newestReleases(b)
	var best *release.Record
	for i := range b.Records {
		r := &b.Records[i]
		if r.Incident == id && (best == nil || r.At.After(best.At)) {
			best = r
		}
	}
	if best == nil {
		return nil
	}
	c := releaseCause(*best, newest[workloadKey(*best)].Equal(best.At))
	return &c
}

// summarizeReleases counts the releases since from and the issues the
// bad ones caused.
func summarizeReleases(b release.Book, from time.Time) *ReleaseSummary {
	out := &ReleaseSummary{BadIncidents: []string{},
		Wrong: []WrongRelease{}}
	for _, r := range b.Records {
		if r.At.Before(from) {
			continue
		}
		out.Total++
		if !r.Verdict.Bad() {
			continue
		}
		out.Bad++
		if len(out.Wrong) < maxWrong {
			out.Wrong = append(out.Wrong, WrongRelease{ID: r.ID,
				Kind: r.Kind, Namespace: r.Namespace, Name: r.Name,
				At: r.At, Verdict: r.Verdict, Incident: r.Incident})
		}
		if r.Incident != "" {
			out.BadIncidents = append(out.BadIncidents, r.Incident)
		}
	}
	return out
}

// stripDetail drops every release's health comparison and error line.
func stripDetail(b *release.Book) {
	for i := range b.Records {
		b.Records[i].Detail = nil
	}
}

// workloadKey names a release's workload.
func workloadKey(r release.Record) string {
	return r.Kind + "/" + r.Namespace + "/" + r.Name
}

// newestReleases is the start of each workload's newest release.
func newestReleases(b release.Book) map[string]time.Time {
	out := map[string]time.Time{}
	for _, r := range b.Records {
		if key := workloadKey(r); r.At.After(out[key]) {
			out[key] = r.At
		}
	}
	return out
}

// releaseCause is r for the page. Only the workload's newest release
// offers an undo command: a rollback goes back one revision from what
// runs now, so on an older release it would bring back the newer one
// (or the bad release a rollback replaced).
func releaseCause(r release.Record, newest bool) ReleaseCause {
	u := r.Undo()
	if !newest && u.Command != "" {
		u = release.Undo{Text: "A newer release of this workload came " +
			"after this one: undo from the newest release"}
	}
	item := ReleaseCause{Record: r, Newest: newest,
		Undo: UndoStep{Text: u.Text, Command: u.Command,
			Mutating: u.Mutating}}
	if !r.RolledOut.IsZero() && !r.RolledOut.Before(r.At) {
		took := r.RolledOut.Sub(r.At).Seconds()
		item.RolloutSeconds = &took
	}
	return item
}

// wrongReleaseIssues is the IDs of the issues that releases of the last
// 24 hours that went wrong caused (?release=bad).
func (s *Server) wrongReleaseIssues(now time.Time) map[string]bool {
	out := map[string]bool{}
	if snap, ok := s.releaseBookSnapshot(); ok {
		for _, r := range snap.Records {
			if r.Verdict.Bad() && r.Incident != "" &&
				!r.At.Before(now.Add(-releaseWindow)) {
				out[r.Incident] = true
			}
		}
	}
	return out
}
