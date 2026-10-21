package compose

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
)

// factClauses start a second clause of a detector summary that states
// the fact itself, not commentary on it: "Service api selects app=apii;
// no pod or workload template in shop has those labels". Dropping it
// would leave "api selects app=apii" with nothing wrong in it.
var factClauses = []string{"no pod or workload template in "}

// factClause is the second clause of a summary when it states the fact,
// else "".
func factClause(after string) string {
	after = strings.TrimSpace(after)
	for _, start := range factClauses {
		if strings.HasPrefix(after, start) {
			return after
		}
	}
	return ""
}

// withoutOwnName drops the entity's own name that follows its kind in a
// summary ("Service api selects ..."): the lead already names it.
func withoutOwnName(summary string, id inventory.EntityID) string {
	if id.Name == "" {
		return summary
	}
	return strings.TrimPrefix(summary, id.Name+" ")
}

// wholeRate matches a number written with a needless ".0" in front of
// the word it counts: "12.0 requests".
var wholeRate = regexp.MustCompile(`(^|[^\w.])(\d+)\.0( [a-z])`)

// wholeNumbers says "12 requests a second" for a count a detector wrote
// as "12.0 requests a second". Versions and quantities ("1.0.0",
// "2.0Gi") are not followed by a space and a word, so they stay.
func wholeNumbers(text string) string {
	return wholeRate.ReplaceAllString(text, "${1}${2}${3}")
}
