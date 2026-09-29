package reason

import (
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

// ChangeSetWindow groups changes to a workload and the configuration it
// references into one release when they happen this close together.
const ChangeSetWindow = 2 * time.Minute

// companionChanges lists the Secrets and ConfigMaps a workload references
// that changed within ChangeSetWindow of change.
func companionChanges(
	q Query, workload knowledge.EntityID, change knowledge.Change,
) []knowledge.EntityID {
	var out []knowledge.EntityID
	for _, ref := range q.Model.Related(
		workload, knowledge.References, knowledge.Outgoing,
	) {
		if ref.Kind != kube.KindSecret && ref.Kind != kube.KindConfigMap {
			continue
		}
		for _, c := range q.Model.Changes(
			ref, change.At.Add(-ChangeSetWindow),
		) {
			if c.At.Sub(change.At) <= ChangeSetWindow && !c.Created {
				out = append(out, ref)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	return out
}

// describeCompanions renders "; configmap app-config and secret db
// changed with it".
func describeCompanions(ids []knowledge.EntityID) string {
	if len(ids) == 0 {
		return ""
	}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, string(id.Kind)+" "+id.Name)
	}
	return "; " + strings.Join(names, " and ") + " changed with it"
}

// rolloutNear reports whether workload rolled out within ChangeSetWindow
// of at, in which case a configuration change belongs to that release.
func rolloutNear(q Query, workload knowledge.EntityID, at time.Time) bool {
	for _, c := range q.Model.Changes(workload, at.Add(-ChangeSetWindow)) {
		if isRollout(c) && c.At.Sub(at) <= ChangeSetWindow {
			return true
		}
	}
	return false
}
