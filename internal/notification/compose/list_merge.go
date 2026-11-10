package compose

import (
	"regexp"
	"strings"
)

// This file merges the titles of a list (a roll-up, a startup summary,
// a digest) that say the same of different workloads in one namespace,
// so the list reads "app0, app1 and app2 in shop keep crashing." instead
// of the same sentence three times.

// sameTitle splits a title that names one workload in its namespace:
// its name, the namespace, the first word of its state and the rest.
var sameTitle = regexp.MustCompile(
	`^([a-z0-9][a-z0-9.-]*) in (\S+) (\S+) (.+)$`)

// mergeTitles joins list sentences that differ only in the workload
// they name, keeping the order in which each first appears. A sentence
// of another shape stays as it is.
func mergeTitles(sentences []sentence) []sentence {
	type group struct {
		at    int
		names []string
		match []string
	}
	groups := map[string]*group{}
	out := make([]sentence, 0, len(sentences))
	for _, s := range sentences {
		m := sameTitle.FindStringSubmatch(s.text)
		if m == nil || pluralVerbs[m[3]] == "" ||
			strings.Contains(" "+m[4]+" ", " it ") {
			// "it" in the state ("since it was created") would not
			// agree with several names.
			out = append(out, s)
			continue
		}
		key := strings.Join(m[2:], " ")
		if g, ok := groups[key]; ok {
			g.names = append(g.names, m[1])
			continue
		}
		groups[key] = &group{at: len(out), names: []string{m[1]}, match: m}
		out = append(out, s)
	}
	for _, g := range groups {
		if len(g.names) < 2 {
			continue
		}
		out[g.at].text = joinAnd(g.names, len(g.names)) + " in " +
			g.match[2] + " " + pluralPredicate(g.match[3]+" "+g.match[4])
	}
	return out
}
