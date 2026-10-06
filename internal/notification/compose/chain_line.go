package compose

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// chainLineSentences are the small print that shows a failure chain: what
// failed after what, with the time each began, such as "Chain: postgres
// (10:00) → api (10:01) → service api (10:01)." A chain of one is no
// chain, so a cause that explains only what it reaches directly has no
// line. Failures one step past the limit are named, so the reader knows
// the chain was cut and not finished.
func chainLineSentences(f caseFacts) []sentence {
	cause := f.p.Cause
	if cause == nil || distinctNames(cause.Hops) < 2 {
		return nil
	}
	steps := make([]string, 0, len(cause.Hops))
	for _, hop := range cause.Hops {
		steps = append(steps, hopWords(hop))
	}
	text := "Chain: " + strings.Join(steps, " → ") + "."
	if len(cause.Beyond) > 0 {
		text += " " + beyondWords(cause.Beyond)
	}
	return []sentence{{part: partChecked, weight: 1, text: text}}
}

// distinctNames counts the different names along a chain. "payments →
// service payments" is one name seen twice (a workload and its own
// Service), so it is not a chain.
func distinctNames(hops []rootcause.Hop) int {
	seen := map[string]bool{}
	for _, hop := range hops {
		seen[hop.Entity.Name] = true
	}
	return len(seen)
}

// hopWords names one hop with the time it began: "api (10:01)".
func hopWords(hop rootcause.Hop) string {
	name := shortName(hop.Entity)
	if hop.Began.IsZero() {
		return name
	}
	return name + " (" + clock(hop.Began) + ")"
}

// beyondWords says what lies past the end of a cut chain.
func beyondWords(beyond []rootcause.Hop) string {
	names := make([]string, 0, len(beyond))
	for _, hop := range beyond {
		names = append(names, hopWords(hop))
	}
	verb := " also fails"
	if len(names) > 1 {
		verb = " also fail"
	}
	return "Beyond that, " + strings.Join(names, " and ") + verb +
		"; kwatch follows a chain at most " +
		strconv.Itoa(explain.ChainMaxHops) +
		" steps, so it is reported on its own."
}
