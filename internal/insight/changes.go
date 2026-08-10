package insight

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/model"
)

func (e *Engine) checkRecentChanges(inc *model.Incident, ins *Insight) {
	if e.tracker == nil {
		return
	}
	recent := e.tracker.RecentChangesBefore(5 * time.Minute)
	filtered := make([]context.Change, 0, len(recent))
	matchingNames := make(map[string]bool)
	for _, k := range graphKeysForIncident(inc) {
		parts := strings.SplitN(k, "/", 3)
		matchingNames[parts[2]] = true
	}
	if len(matchingNames) == 0 {
		matchingNames[inc.Name] = true
	}
	for _, c := range recent {
		if c.Resource == inc.Resource && c.Namespace == inc.Namespace && matchingNames[c.Name] {
			filtered = append(filtered, c)
		}
	}
	if len(filtered) > 0 {
		ins.RecentChanges = filtered
		return
	}

	// Dependency-filtered pass: check if any dependency of this resource changed
	if e.graph != nil {
		deps := dependenciesFor(e.graph, inc)
		if len(deps) > 0 {
			depChanges := make([]context.Change, 0, len(recent))
			for _, c := range recent {
				depKey := c.Resource + "/" + c.Namespace + "/" + c.Name
				for _, d := range deps {
					if d == depKey && c.Type == context.ChangeUpdate {
						depChanges = append(depChanges, c)
						break
					}
				}
			}
			if len(depChanges) > 0 {
				if len(depChanges) > 3 {
					depChanges = depChanges[:3]
				}
				ins.RecentChanges = depChanges
				// Only enhance the cause with dependency-change info when no
				// more specific diagnosis was produced (e.g. node_failure or
				// rollout_failure); otherwise the generic update wording would
				// hide the actual root cause.
				if ins.Cause == "" && ins.Pattern == "" {
					c := depChanges[0]
					delta := time.Since(c.Timestamp).Round(time.Second)
					if delta < 0 {
						delta = 0
					}
					ins.Cause = fmt.Sprintf("%s %s/%s was updated %s before this incident",
						c.Resource, c.Namespace, c.Name, delta)
					ins.Pattern = "dependency_change"
				}
				return
			}
		}
	}

	for _, c := range recent {
		if c.Namespace == inc.Namespace {
			filtered = append(filtered, c)
		}
	}
	if len(filtered) > 3 {
		filtered = filtered[:3]
	}
	if len(filtered) > 0 {
		ins.RecentChanges = filtered
	}
}
