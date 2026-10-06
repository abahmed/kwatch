package compose

import "strings"

// changesBeforeSentences name what else changed near the root before it
// broke, for an incident that has a cause: the cause's own change is
// already said, these are the neighbours, nearest first. Without a
// cause, recentChangeSentence says the same under the lead.
func changesBeforeSentences(f caseFacts) []sentence {
	if f.p.Cause == nil || len(f.changes) == 0 {
		return nil
	}
	items := make([]string, 0, len(f.changes))
	for _, change := range f.changes {
		items = append(items, changeFact(change))
	}
	return []sentence{{part: partChanges, weight: 0,
		text: "Also changed before it broke: " +
			strings.Join(items, "; ") + "."}}
}
