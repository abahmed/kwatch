package dashboard

import (
	"cmp"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// Trends cover the last trendDays calendar days, today included, and
// list at most trendTop namespaces and noisy issues.
const (
	trendDays = 7
	trendTop  = 5
	// maxTZOffset bounds ?tz=, minutes east of UTC: UTC-14 to UTC+14.
	maxTZOffset = 14 * 60
	// trendEnough is how many issues opened or resolved in the week make
	// the trends worth showing.
	trendEnough = 2
)

// TrendsResponse is the /api/trends response: what the issues of the
// last seven days looked like, from the incidents kwatch still holds.
// PreviousWeek is how many opened in the seven days before, or nil when
// the incidents kept do not cover that week. Enough is false when there
// is too little to draw.
type TrendsResponse struct {
	Days         []TrendDay       `json:"days"`
	Opened       int              `json:"opened"`
	PreviousWeek *int             `json:"previousWeek,omitempty"`
	Namespaces   []TrendNamespace `json:"namespaces"`
	Noisiest     []TrendIssue     `json:"noisiest"`
	Resolve      TrendResolve     `json:"resolve"`
	Enough       bool             `json:"enough"`
}

// TrendDay is how many issues opened on one calendar day (YYYY-MM-DD
// in the requested zone), by the issue's current severity.
type TrendDay struct {
	Date   string `json:"date"`
	Page   int    `json:"page"`
	Notify int    `json:"notify"`
	Digest int    `json:"digest"`
	Total  int    `json:"total"`
}

// TrendNamespace is how many times issues opened in a namespace this
// week; Active is how many of its issues are not resolved yet.
type TrendNamespace struct {
	Namespace string `json:"namespace"`
	Count     int    `json:"count"`
	Active    int    `json:"active"`
}

// TrendIssue is an issue that opened again and again this week, or is
// flapping now.
type TrendIssue struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Severity  string `json:"severity"`
	State     string `json:"state"`
	Namespace string `json:"namespace"`
	Workload  string `json:"workload"`
	// WorkloadKind is the kind of the root object, as on list items.
	WorkloadKind string `json:"workloadKind,omitempty"`
	Opened       int    `json:"opened"`
	RepeatCount  int    `json:"repeatCount"`
}

// TrendResolve is how long the issues resolved this week took: the
// median and the longest, in seconds, and the issue that took longest.
type TrendResolve struct {
	Count          int    `json:"count"`
	MedianSeconds  int64  `json:"medianSeconds"`
	LongestSeconds int64  `json:"longestSeconds"`
	LongestID      string `json:"longestId,omitempty"`
}

// trendsHandler serves GET /api/trends?tz=, tz being the minutes east
// of UTC that days are counted in (default 0, UTC).
func (s *Server) trendsHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	tz, ok := parseTZ(r.URL.Query().Get("tz"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid tz")
		return
	}
	reader := s.sources.incidents.Load()
	if reader == nil {
		unavailable(w)
		return
	}
	writeTagged(w, r, buildTrends((*reader).Incidents(), s.now.Now(), tz,
		incident.DefaultRemember))
}

// parseTZ reads ?tz=: empty is UTC, anything else must be whole minutes
// within maxTZOffset of UTC.
func parseTZ(raw string) (int, bool) {
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < -maxTZOffset || n > maxTZOffset {
		return 0, false
	}
	return n, true
}

// trendWeek is the week trends count in: trendDays calendar days of
// zone ending with today, and the same length before it.
type trendWeek struct {
	zone      *time.Location
	start     time.Time
	end       time.Time
	prevStart time.Time
	now       time.Time
}

func newTrendWeek(now time.Time, tz int) trendWeek {
	zone := time.FixedZone("", tz*60)
	local := now.In(zone)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0,
		0, zone)
	start := today.AddDate(0, 0, 1-trendDays)
	return trendWeek{zone: zone, start: start, end: today.AddDate(0, 0, 1),
		prevStart: start.AddDate(0, 0, -trendDays), now: now}
}

// day is the index of t's day in the week, or -1 when t is outside.
func (w trendWeek) day(t time.Time) int {
	if t.Before(w.start) || !t.Before(w.end) {
		return -1
	}
	return int(t.Sub(w.start) / (24 * time.Hour))
}

// openings are the times p opened: every occurrence kwatch kept across
// reopens and recurrences, or Opened alone for a record without them.
func openings(p incident.Incident) []time.Time {
	if len(p.Occurrences) > 0 {
		return p.Occurrences
	}
	if p.Opened.IsZero() {
		return nil
	}
	return []time.Time{p.Opened}
}

// buildTrends aggregates items as of now, counting days tz minutes east
// of UTC. remember is how long resolved incidents are kept: the week
// before this one is compared only when that covers it. Silent
// incidents are left out, as on the active list.
func buildTrends(
	items []incident.Incident, now time.Time, tz int,
	remember time.Duration,
) TrendsResponse {
	w := newTrendWeek(now, tz)
	out := TrendsResponse{Days: make([]TrendDay, trendDays),
		Namespaces: make([]TrendNamespace, 0),
		Noisiest:   make([]TrendIssue, 0)}
	for i := range out.Days {
		out.Days[i].Date = w.start.AddDate(0, 0, i).Format(time.DateOnly)
	}
	spaces := map[string]*TrendNamespace{}
	var durations []trendDuration
	previous, covered := 0, false
	for _, p := range items {
		if p.Tier == incident.Silent {
			continue
		}
		n := 0
		for _, at := range openings(p) {
			if !at.After(w.prevStart) {
				covered = true
			}
			if !at.Before(w.prevStart) && at.Before(w.start) {
				previous++
			}
			if i := w.day(at); i >= 0 {
				out.Days[i].add(p.Tier)
				n++
			}
		}
		out.Opened += n
		countSpace(spaces, p, n)
		if n >= 2 || p.State == incident.Flapping {
			out.Noisiest = append(out.Noisiest, noisy(p, n))
		}
		durations = append(durations, resolvedIn(p, w)...)
	}
	if covered && remember >= 2*trendDays*24*time.Hour {
		out.PreviousWeek = &previous
	}
	out.Namespaces = topSpaces(spaces)
	slices.SortFunc(out.Noisiest, noisier)
	out.Noisiest = out.Noisiest[:min(len(out.Noisiest), trendTop)]
	out.Resolve = summarize(durations)
	out.Enough = out.Opened >= trendEnough ||
		out.Resolve.Count >= trendEnough
	return out
}

func (d *TrendDay) add(t incident.Tier) {
	switch t {
	case incident.Page:
		d.Page++
	case incident.Notify:
		d.Notify++
	default:
		d.Digest++
	}
	d.Total++
}

func countSpace(spaces map[string]*TrendNamespace, p incident.Incident,
	n int) {
	ns := p.Root.Namespace
	if ns == "" || n == 0 {
		return
	}
	s := spaces[ns]
	if s == nil {
		s = &TrendNamespace{Namespace: ns}
		spaces[ns] = s
	}
	s.Count += n
	if p.State != incident.Resolved {
		s.Active++
	}
}

// topSpaces is the trendTop namespaces with the most openings, ties
// A to Z.
func topSpaces(spaces map[string]*TrendNamespace) []TrendNamespace {
	out := make([]TrendNamespace, 0, len(spaces))
	for _, s := range spaces {
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b TrendNamespace) int {
		return cmp.Or(b.Count-a.Count, cmp.Compare(a.Namespace, b.Namespace))
	})
	return out[:min(len(out), trendTop)]
}

func noisy(p incident.Incident, n int) TrendIssue {
	item := toListItem(p)
	return TrendIssue{ID: item.ID, Title: item.Title,
		Severity: item.Severity, State: item.State,
		Namespace: item.Namespace, Workload: item.Workload,
		WorkloadKind: item.WorkloadKind, Opened: n,
		RepeatCount: p.RepeatCount}
}

// noisier orders the most openings first, then flapping issues, then the
// most repeats, then by ID so the order is stable.
func noisier(a, b TrendIssue) int {
	flap := func(t TrendIssue) int {
		if t.State == incident.Flapping.String() {
			return 0
		}
		return 1
	}
	return cmp.Or(b.Opened-a.Opened, flap(a)-flap(b),
		b.RepeatCount-a.RepeatCount, cmp.Compare(a.ID, b.ID))
}

// trendDuration is how long one resolved occurrence of an issue took.
type trendDuration struct {
	id   string
	took time.Duration
}

// resolvedIn lists the occurrences of p resolved within the week: its
// earlier occurrences from History and p itself when it is resolved.
func resolvedIn(p incident.Incident, w trendWeek) []trendDuration {
	var out []trendDuration
	add := func(opened, resolved time.Time) {
		if opened.IsZero() || resolved.Before(opened) ||
			resolved.Before(w.start) || resolved.After(w.now) {
			return
		}
		out = append(out, trendDuration{id: p.ID,
			took: resolved.Sub(opened)})
	}
	for _, o := range p.History {
		add(o.Opened, o.Resolved)
	}
	if p.State == incident.Resolved {
		add(p.Opened, p.Resolved)
	}
	return out
}

// summarize is the median and the longest of ds; the first issue wins
// a tie for the longest, by ID.
func summarize(ds []trendDuration) TrendResolve {
	if len(ds) == 0 {
		return TrendResolve{}
	}
	slices.SortFunc(ds, func(a, b trendDuration) int {
		return cmp.Or(cmp.Compare(a.took, b.took), cmp.Compare(b.id, a.id))
	})
	mid := len(ds) / 2
	median := ds[mid].took
	if len(ds)%2 == 0 {
		median = (ds[mid-1].took + ds[mid].took) / 2
	}
	last := ds[len(ds)-1]
	return TrendResolve{Count: len(ds),
		MedianSeconds:  int64(median / time.Second),
		LongestSeconds: int64(last.took / time.Second), LongestID: last.id}
}
