package compose

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
)

// maxReachNamed is how many addresses a message names; the rest are
// counted ("and 2 more"). The dashboard lists them.
const maxReachNamed = 2

// reachSentences say where users reach what is affected, after the
// object line that names the Ingress or Service: "Users can't reach
// shop.example.com/checkout and 2 more." Addresses with no ready
// endpoint come first. When only some endpoints are gone the sentence
// counts them for the worst address: "1 of 3 endpoints ready for
// shop.example.com/checkout." Nothing is said when no Ingress or route
// is found: a guess would name an address nobody published.
func reachSentences(f caseFacts) []sentence {
	var down []incident.Reach
	for _, r := range f.reach {
		if r.Down() {
			down = append(down, r)
		}
	}
	switch {
	case len(down) > 0:
		return []sentence{{part: partConsequence, weight: 0.9, keepCase: true,
			text: "Users can't reach " + reachList(down) + "."}}
	case len(f.reach) > 0 && f.reach[0].Total > f.reach[0].Ready:
		r := f.reach[0]
		return []sentence{{part: partConsequence, weight: 0.9, keepCase: true,
			text: strconv.Itoa(r.Ready) + " of " + strconv.Itoa(r.Total) +
				" endpoints ready for " + r.Address() + "."}}
	}
	return nil
}

// reachList names the first maxReachNamed addresses and counts the rest.
func reachList(reach []incident.Reach) string {
	named := min(len(reach), maxReachNamed)
	names := make([]string, named)
	for i := range named {
		names[i] = reach[i].Address()
	}
	text := strings.Join(names, ", ")
	if rest := len(reach) - named; rest > 0 {
		text = joinNames(names) + " and " + strconv.Itoa(rest) + " more"
	} else if named == 2 {
		text = names[0] + " and " + names[1]
	}
	return text
}

// joinNames joins names with commas.
func joinNames(names []string) string { return strings.Join(names, ", ") }
