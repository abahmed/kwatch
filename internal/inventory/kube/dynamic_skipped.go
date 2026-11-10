package kube

import (
	"sort"
	"strings"
)

// skippedChanged returns the skipped kinds and whether they differ from
// the list last logged, so the log names them at startup and again only
// when the list changes, not on every discovery pass. An empty list is
// never logged.
func (d *DynamicSource) skippedChanged(
	skipped []plannedResource,
) (string, bool) {
	kinds := skippedKinds(skipped)
	d.mu.Lock()
	defer d.mu.Unlock()
	changed := kinds != "" && kinds != d.loggedSkipped
	d.loggedSkipped = kinds
	return kinds, changed
}

// skippedKinds names the kinds the budget left out, sorted, so an
// operator can raise the budget or recognise a gap without guessing. The
// list is bounded by what discovery returned beyond the budget.
func skippedKinds(skipped []plannedResource) string {
	names := make([]string, 0, len(skipped))
	for _, r := range skipped {
		names = append(names, r.kind)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}
