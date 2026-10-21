package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// limitFields are the bounds a LimitRange item sets, in the order the
// change text lists them.
var limitFields = []string{"min", "max", "default", "defaultRequest",
	"ratio"}

// limitRangeValues words a LimitRange change by the bounds that changed:
// "Container max cpu=4" to "Container max cpu=1". The inventory writes
// every bound of every item, empty ones too ("min ; max cpu=4; default
// ; ..."); only the ones that differ are worth a reader's time.
func limitRangeValues(before, after string) (string, string) {
	old, now := limitItems(before), limitItems(after)
	var from, to []string
	for _, kind := range limitKinds(old, now) {
		for _, field := range limitFields {
			was, is := old[kind][field], now[kind][field]
			if was == is {
				continue
			}
			if was != "" {
				from = append(from, kind+" "+field+" "+was)
			}
			if is != "" {
				to = append(to, kind+" "+field+" "+is)
			}
		}
	}
	return orNothing(from), orNothing(to)
}

func orNothing(parts []string) string {
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, "; ")
}

// limitItems reads the inventory's text: items split by " | ", each a
// type and its bounds split by "; ", each "name value" with the value
// empty when the bound is not set.
func limitItems(text string) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, item := range strings.Split(text, " | ") {
		kind, bounds, _ := strings.Cut(item, " ")
		if kind == "" {
			continue
		}
		out[kind] = map[string]string{}
		for _, bound := range strings.Split(bounds, "; ") {
			name, value, _ := strings.Cut(strings.TrimSpace(bound), " ")
			out[kind][name] = strings.TrimSpace(value)
		}
	}
	return out
}

// limitKinds is the item types of both texts, sorted and without
// repeats.
func limitKinds(a, b map[string]map[string]string) []string {
	seen := map[string]bool{}
	for kind := range a {
		seen[kind] = true
	}
	for kind := range b {
		seen[kind] = true
	}
	return sortedSet(seen)
}

// limitRangeChange reports the one field of a LimitRange change whose
// text is the inventory's limits text.
func limitRangeChange(change inventory.Change) bool {
	return change.Entity.Kind == kube.KindLimitRange &&
		len(change.Fields) == 1 &&
		lastSegment(change.Fields[0].Path) == "limits"
}
