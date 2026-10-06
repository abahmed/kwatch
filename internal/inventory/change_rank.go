package inventory

import (
	"sort"
	"strings"
)

// culpritRule ranks a field path by how often an edit of it breaks a
// workload. Rules are checked in order; the first match wins.
type culpritRule struct {
	contains string
	rank     int
}

// culpritRules order the usual suspects of a bad release: the image, then
// the environment, then what the container runs, then its limits, its
// probes, and last the volumes and config it loads.
var culpritRules = []culpritRule{
	{"].image", 10},
	{".env.", 9},
	{".command", 8},
	{".args", 8},
	{"resources.limits", 7},
	{"resources.requests", 6},
	{"Probe", 5},
	{"volumes[", 4},
	{".envFrom", 4},
	{"references", 4},
	{".ports", 3},
}

// FieldRank says how likely an edit of path is to be the cause of a
// failure that follows it: higher is more likely. Paths no rule knows
// rank 1.
func FieldRank(path string) int {
	for _, rule := range culpritRules {
		if strings.Contains(path, rule.contains) {
			return rule.rank
		}
	}
	return 1
}

// RankFields returns fields ordered from the most to the least likely
// culprit. Fields of equal rank keep their order.
func RankFields(fields []FieldChange) []FieldChange {
	out := append([]FieldChange(nil), fields...)
	sort.SliceStable(out, func(i, j int) bool {
		return FieldRank(out[i].Path) > FieldRank(out[j].Path)
	})
	return out
}
