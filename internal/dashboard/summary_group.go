package dashboard

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// groupObjects are the objects of an issue that groups several: what a
// removed controller left behind (incident.Leftovers), else the
// failing members, other than the root, that have the issue's
// symptom (symptomMembers). The title's count, "What's wrong?" and the
// list all come from it, so they never disagree.
func groupObjects(p incident.Incident) []inventory.EntityID {
	members, ok := p.Leftovers()
	if !ok {
		return symptomMembers(p, modeFamily(p))
	}
	seen := map[inventory.EntityID]bool{}
	var out []inventory.EntityID
	for _, m := range members {
		if !seen[m.Entity] {
			seen[m.Entity] = true
			out = append(out, m.Entity)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	return out
}

// countWords are the counts a title spells out, as messages write
// them.
var countWords = []string{"zero", "one", "two", "three", "four", "five",
	"six", "seven", "eight", "nine", "ten"}

// Count phrases of a group title: "(4 objects)" anywhere, or a title
// that opens with "Two objects", the names of a few after it.
var (
	countInBrackets = regexp.MustCompile(`\((\w+) objects\)`)
	countAtStart    = regexp.MustCompile(`^(\w+) objects( \([^)]*\))?`)
)

// liveTitle is a stored title with its count of objects made the
// current one. A title is written once (incident.Problem) while its
// group grows or shrinks, so "(4 objects)" can name a set that has
// since become 2: the count is set to n, and the names that a stale
// count came with are left out, since they no longer name the set.
// title is unchanged when it has no count, the counts agree, or
// either is below two.
func liveTitle(title string, n int) string {
	if n < 2 {
		return title
	}
	if m := countInBrackets.FindStringSubmatchIndex(title); m != nil {
		if was := countOf(title[m[2]:m[3]]); was > 1 && was != n {
			return title[:m[2]] + strconv.Itoa(n) + title[m[3]:]
		}
		return title
	}
	m := countAtStart.FindStringSubmatchIndex(title)
	if m == nil {
		return title
	}
	if was := countOf(title[m[2]:m[3]]); was > 1 && was != n {
		return upperFirst(spelled(n)) + " objects" + title[m[1]:]
	}
	return title
}

// countOf reads a count written as digits or spelled out; 0 when it
// is neither.
func countOf(word string) int {
	if n, err := strconv.Atoi(word); err == nil {
		return n
	}
	for i, w := range countWords {
		if strings.EqualFold(w, word) {
			return i
		}
	}
	return 0
}

// spelled is a count as a title writes it: small ones in words.
func spelled(n int) string {
	if n >= 0 && n < len(countWords) {
		return countWords[n]
	}
	return strconv.Itoa(n)
}

// groupTitle is p's title with the live count of its group.
func groupTitle(p incident.Incident, title string) string {
	if !countInBrackets.MatchString(title) &&
		!countAtStart.MatchString(title) {
		return title
	}
	return liveTitle(title, len(groupObjects(p)))
}
