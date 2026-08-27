package insight

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/model"
)

// directResourceChanges returns recent tracker entries matching the incident's
// own resource identity.
func directResourceChanges(
	recent []context.Change,
	inc *model.Incident,
) []context.Change {
	matchingNames := make(map[string]bool)
	for _, k := range graphKeysForIncident(inc) {
		parts := strings.SplitN(k, "/", 3)
		matchingNames[parts[2]] = true
	}
	if len(matchingNames) == 0 {
		matchingNames[inc.Name] = true
	}
	filtered := make([]context.Change, 0, len(recent))
	for _, c := range recent {
		if c.Resource == inc.Resource && c.Namespace == inc.Namespace &&
			matchingNames[c.Name] {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// appendDependencyChanges records updates to the incident's dependencies and,
// absent a more specific diagnosis, attributes the cause to the newest change.
// Reports whether any dependency changes were found.
func (e *Engine) appendDependencyChanges(
	recent []context.Change,
	inc *model.Incident,
	ins *Insight,
) bool {
	deps := dependenciesFor(e.graph, inc)
	if len(deps) == 0 {
		return false
	}
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
	if len(depChanges) == 0 {
		return false
	}
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
		ins.Cause = fmt.Sprintf("%s %s/%s was updated %s before this incident",
			c.Resource, c.Namespace, c.Name, ageOf(c.Timestamp, e.now()))
		ins.Pattern = "dependency_change"
	}
	return true
}

// workloadKinds are the owners whose spec change is a rollout.
var workloadKinds = map[string]bool{
	"deployment":  true,
	"statefulset": true,
	"daemonset":   true,
}

// ownerRolloutChanges finds recent updates to the workload that owns the
// failing pods. A Deployment updated two minutes before its pods stopped
// being ready is the single most useful thing an alert can say.
func ownerRolloutChanges(
	recent []context.Change,
	inc *model.Incident,
) []context.Change {
	if inc.Resource != "pod" || inc.OwnerKind == "" {
		return nil
	}
	kind := strings.ToLower(inc.OwnerKind)
	if !workloadKinds[kind] {
		return nil
	}
	owner := strings.TrimPrefix(inc.Name, inc.Namespace+"/")
	var out []context.Change
	for _, c := range recent {
		if c.Resource == kind && c.Namespace == inc.Namespace &&
			c.Name == owner &&
			c.Type == context.ChangeUpdate {
			out = append(out, c)
		}
	}
	return out
}

func (e *Engine) checkRecentChanges(inc *model.Incident, ins *Insight) {
	if e.tracker == nil {
		return
	}
	recent := e.tracker.RecentChangesBefore(5 * time.Minute)

	// A pod's own create/update/delete is the incident, not its cause;
	// listing it as "what changed" says nothing. Look at what the pods
	// depend on instead: their owner's rollout, then their config.
	if inc.Resource == "pod" {
		if rollout := ownerRolloutChanges(recent, inc); len(rollout) > 0 {
			ins.RecentChanges = rollout[:min(len(rollout), 3)]
			if ins.Cause == "" {
				c := rollout[0]
				ins.Cause = fmt.Sprintf(
					"%s %s/%s was updated %s before this incident — likely a "+
						"rollout",
					inc.OwnerKind,
					c.Namespace,
					c.Name,
					ageOf(c.Timestamp, e.now()),
				)
				ins.Pattern = "rollout"
			}
			return
		}
	} else {
		filtered := directResourceChanges(recent, inc)
		if len(filtered) > 0 {
			ins.RecentChanges = filtered[:min(len(filtered), 3)]
			return
		}
	}

	// Dependency-filtered pass: check if any dependency of this resource
	// changed
	if e.graph != nil && e.appendDependencyChanges(recent, inc, ins) {
		return
	}

	// Namespace-wide fallback, capped at three entries.
	// No namespace-wide fallback: unrelated churn in the same namespace is
	// noise, and noise here undermines the cases where the change is real.
}

// ageOf renders how long before now t was, coarsely — "2m", "40s", "3h".
func ageOf(t, now time.Time) string {
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}
