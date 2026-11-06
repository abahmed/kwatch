package dashboard

import (
	"net/http"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/release"
)

// releaseWindows are the windows /api/releases takes.
var releaseWindows = map[string]time.Duration{
	"24h": 24 * time.Hour, "7d": release.KeepFor,
}

// releaseVerdicts are the verdict filters: one verdict, or "bad" for
// every release that went wrong.
var releaseVerdicts = map[string]bool{"": true, "bad": true,
	string(release.Rolling): true, string(release.Watching): true,
	string(release.Healthy): true, string(release.Regressed): true,
	string(release.Failed): true, string(release.Down): true,
	string(release.Replaced): true, string(release.Unknown): true}

// ReleasesResponse is the /api/releases response.
type ReleasesResponse struct {
	Since string `json:"since"`
	// Total counts the releases in the window before the namespace and
	// verdict filters; Bad the ones of them that went wrong.
	Total int `json:"total"`
	Bad   int `json:"bad"`
	// Namespaces are those with releases in the window, for the filter.
	Namespaces []string      `json:"namespaces"`
	Releases   []ReleaseItem `json:"releases"`
}

// ReleaseItem is one release on the Releases page.
type ReleaseItem struct {
	release.Record
	// RolloutSeconds is how long the rollout took, once it finished.
	RolloutSeconds *float64 `json:"rolloutSeconds,omitempty"`
	Undo           UndoStep `json:"undo"`
}

// UndoStep is how to undo a release: a text and, when one does it, the
// command.
type UndoStep struct {
	Text     string `json:"text"`
	Command  string `json:"command,omitempty"`
	Mutating bool   `json:"mutating,omitempty"`
}

// releasesHandler serves GET /api/releases?since=24h|7d&namespace=
// &verdict=: the releases kwatch saw, newest first.
func (s *Server) releasesHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	q := r.URL.Query()
	since := q.Get("since")
	if since == "" {
		since = "7d"
	}
	window, ok := releaseWindows[since]
	if !ok || len(q["since"]) > 1 {
		writeBadQuery(w, badQuery("since", since,
			"Unknown releases window: %q. Use 24h or 7d.", since))
		return
	}
	verdict := q.Get("verdict")
	if !releaseVerdicts[verdict] || len(q["verdict"]) > 1 {
		writeBadQuery(w, badQuery("verdict", verdict,
			"Unknown verdict: %q.", verdict))
		return
	}
	namespace := q.Get("namespace")
	if len(q["namespace"]) > 1 {
		writeBadQuery(w, badQuery("namespace", namespace,
			"Pick one namespace, not %d.", len(q["namespace"])))
		return
	}
	book := s.sources.releases.Load()
	if book == nil {
		unavailable(w)
		return
	}
	snap := book.snapshot()
	if !s.opts.ChangeHistory {
		// Detail is Pro: a book kept from a Pro run serves none of it.
		stripDetail(&snap)
	}
	out := buildReleases(snap, s.now.Now().Add(-window),
		namespace, verdict)
	out.Since = since
	writeTagged(w, r, out)
}

// buildReleases lists the releases since from, counting the window
// before the namespace and verdict filters.
func buildReleases(
	b release.Book, from time.Time, namespace, verdict string,
) ReleasesResponse {
	out := ReleasesResponse{Namespaces: []string{},
		Releases: []ReleaseItem{}}
	spaces := map[string]bool{}
	newest := newestReleases(b)
	for _, r := range b.Records {
		if r.At.Before(from) {
			continue
		}
		out.Total++
		if r.Verdict.Bad() {
			out.Bad++
		}
		spaces[r.Namespace] = true
		if (namespace != "" && r.Namespace != namespace) ||
			!verdictMatches(r.Verdict, verdict) {
			continue
		}
		out.Releases = append(out.Releases,
			releaseItem(r, newest[workloadKey(r)].Equal(r.At)))
	}
	for ns := range spaces {
		out.Namespaces = append(out.Namespaces, ns)
	}
	sort.Strings(out.Namespaces)
	return out
}

func verdictMatches(v release.Verdict, filter string) bool {
	switch filter {
	case "":
		return true
	case "bad":
		return v.Bad()
	}
	return string(v) == filter
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

// releaseItem is r for the page. Only the workload's newest release
// offers an undo command: a rollback goes back one revision from what
// runs now, so on an older release it would bring back the newer one
// (or the bad release a rollback replaced).
func releaseItem(r release.Record, newest bool) ReleaseItem {
	u := r.Undo()
	if !newest && u.Command != "" {
		u = release.Undo{Text: "A newer release of this workload came " +
			"after this one: undo from the newest release"}
	}
	item := ReleaseItem{Record: r, Undo: UndoStep{Text: u.Text,
		Command: u.Command, Mutating: u.Mutating}}
	if !r.RolledOut.IsZero() && !r.RolledOut.Before(r.At) {
		took := r.RolledOut.Sub(r.At).Seconds()
		item.RolloutSeconds = &took
	}
	return item
}
