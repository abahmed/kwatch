package insight

import (
	"sort"
	"time"

	context "github.com/abahmed/kwatch/internal/graphcontext"
)

// rankRootsByEvidence makes graph traversal time-aware. Depth is useful, but
// a resource that changed immediately before an incident is stronger evidence
// than an unchanged leaf.
func (e *Engine) rankRootsByEvidence(roots []modelCauseRef) {
	for i := range roots {
		roots[i].score = roots[i].depth * 10
		if e.tracker == nil {
			continue
		}
		for _, change := range e.tracker.RecentChangesBeforeAt(
			10*time.Minute, e.now(),
		) {
			if !sameRoot(change, roots[i]) {
				continue
			}
			score := recencyScore(change.Timestamp, e.now())
			if change.Type != context.ChangeUpdate {
				score /= 2
			}
			roots[i].score += score
		}
	}
	sort.SliceStable(roots, func(i, j int) bool {
		if roots[i].score != roots[j].score {
			return roots[i].score > roots[j].score
		}
		return roots[i].depth > roots[j].depth
	})
}

func sameRoot(change context.Change, root modelCauseRef) bool {
	return change.Resource == root.Kind &&
		change.Namespace == root.Namespace && change.Name == root.Name
}

func recencyScore(timestamp, now time.Time) int {
	age := now.Sub(timestamp)
	if age < 0 {
		age = 0
	}
	switch {
	case age <= time.Minute:
		return 50
	case age <= 3*time.Minute:
		return 35
	case age <= 7*time.Minute:
		return 20
	default:
		return 10
	}
}
