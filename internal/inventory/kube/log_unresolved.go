package kube

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// A workload that keeps failing to resolve a name shaped like a
// Service of the cluster ("cache", "cache.data", "cache.data.svc…")
// while its pods stay Ready. The sample round notes such names in each
// window it reads; only a name seen in several windows, spread over a
// while and seen lately, is recorded, so one miss at start-up never
// counts. Whether the Service exists is the detector's question.

// Attributes the log sample round writes for such a name.
const (
	// AttrLogUnresolvedName is the Service the log keeps failing to
	// resolve, as "name.namespace".
	AttrLogUnresolvedName = "log.unresolved.name"
	// AttrLogUnresolvedSamples is how many sample windows showed it.
	AttrLogUnresolvedSamples = "log.unresolved.samples"
	// AttrLogUnresolvedSince is the first window that showed it.
	AttrLogUnresolvedSince = "log.unresolved.since"
	// AttrLogUnresolvedLine is a line that showed it, clipped.
	AttrLogUnresolvedLine = "log.unresolved.line"
)

// Persistence of an unresolved name.
const (
	// minUnresolvedSamples is how many sample windows must show it.
	minUnresolvedSamples = 3
	// minUnresolvedSpan is how far apart the first and last must be.
	minUnresolvedSpan = 20 * time.Minute
	// unresolvedQuiet is how long without it clears it.
	unresolvedQuiet = 30 * time.Minute
)

// unresolvedPatterns find, in lower-case text, the name a lookup that
// was answered "no such name" asked for, in the words of common
// resolvers and clients. Group 1 is the name. A lookup that got no
// answer (a timeout, a server failure) is not here: it says nothing
// about the name. Adding a client's words is adding a row.
var unresolvedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`lookup ([a-z0-9][a-z0-9.-]*)(?: on \S+)?: ` +
		`no such host`),
	regexp.MustCompile(`getaddrinfo enotfound ([a-z0-9][a-z0-9.-]*)`),
	regexp.MustCompile(`unknownhostexception: ([a-z0-9][a-z0-9.-]*)`),
	regexp.MustCompile(`name or service not known \(` +
		`([a-z0-9][a-z0-9.-]*)(?::\d+)?\)`),
	regexp.MustCompile(`(?:^|[\s"'(])([a-z0-9][a-z0-9.-]*)(?::\d+)?: ` +
		`name or service not known`),
	regexp.MustCompile(`could not resolve host:? ([a-z0-9][a-z0-9.-]*)`),
}

// unresolvedHints are words every line unresolvedPatterns reads holds;
// a line without one is skipped before any pattern runs.
var unresolvedHints = []string{"not known", "no such host", "notfound",
	"unknownhost", "resolve host"}

// unresolvedIn returns, per Service ("name.namespace"), a line of the
// window that failed to resolve it.
func unresolvedIn(lines []string, namespace string) map[string]string {
	out := map[string]string{}
	for _, line := range lines {
		ns, name, ok := unresolvedService(strings.ToLower(line), namespace)
		if !ok || len(out) >= maxTrackedLines {
			continue
		}
		if _, seen := out[name+"."+ns]; !seen {
			out[name+"."+ns] = clipLine(line)
		}
	}
	return out
}

// unresolvedService is the Service a lower-case line failed to
// resolve, read by the first pattern that matches.
func unresolvedService(lower, namespace string) (string, string, bool) {
	hinted := false
	for _, hint := range unresolvedHints {
		hinted = hinted || strings.Contains(lower, hint)
	}
	if !hinted {
		return "", "", false
	}
	for _, pattern := range unresolvedPatterns {
		if m := pattern.FindStringSubmatch(lower); m != nil {
			return ClusterServiceName(m[1], namespace)
		}
	}
	return "", "", false
}

// unresolvedSighting is what the windows say of one name.
type unresolvedSighting struct {
	name        string
	first, last time.Time
	samples     int
	line        string
}

// persistentUnresolved is the name the windows, oldest first, show
// often enough, over long enough and lately enough: the one in the most
// windows, then the first by name. ok is false when none qualifies.
func persistentUnresolved(
	windows []windowCount, now time.Time,
) (unresolvedSighting, bool) {
	byName := map[string]*unresolvedSighting{}
	for _, w := range windows {
		for name, line := range w.unresolved {
			s := byName[name]
			if s == nil {
				s = &unresolvedSighting{name: name, first: w.at}
				byName[name] = s
			}
			s.last, s.line = w.at, line
			s.samples++
		}
	}
	var best []unresolvedSighting
	for _, s := range byName {
		if s.samples >= minUnresolvedSamples &&
			s.last.Sub(s.first) >= minUnresolvedSpan &&
			now.Sub(s.last) <= unresolvedQuiet {
			best = append(best, *s)
		}
	}
	if len(best) == 0 {
		return unresolvedSighting{}, false
	}
	sort.Slice(best, func(i, j int) bool {
		if best[i].samples != best[j].samples {
			return best[i].samples > best[j].samples
		}
		return best[i].name < best[j].name
	})
	return best[0], true
}

// addUnresolved records the persistent name, if any, on attrs.
func addUnresolved(
	attrs map[string]inventory.Value, windows []windowCount, now time.Time,
) {
	s, ok := persistentUnresolved(windows, now)
	if !ok {
		return
	}
	attrs[AttrLogUnresolvedName] = inventory.Text(s.name)
	attrs[AttrLogUnresolvedSamples] = inventory.Number(float64(s.samples))
	attrs[AttrLogUnresolvedSince] = inventory.Time(s.first)
	attrs[AttrLogUnresolvedLine] = inventory.Text(s.line)
}
