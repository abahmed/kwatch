package insight

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/model"
)

type Insight struct {
	Cause         string
	Impact        string
	Pattern       string
	AffectedCount int
	RecentChanges []context.Change
}

type Engine struct {
	graph   *context.ResourceGraph
	tracker *context.ChangeTracker
	// now is the clock "updated 3m ago" is measured against; injectable so
	// tests do not depend on the wall clock.
	now func() time.Time
}

func NewEngine(
	graph *context.ResourceGraph,
	tracker *context.ChangeTracker,
) *Engine {
	return &Engine{graph: graph, tracker: tracker, now: time.Now}
}

func (e *Engine) Analyze(inc *model.Incident) *Insight {
	ins := &Insight{}

	e.determineCause(inc, ins)
	e.describeImpact(inc, ins)
	e.checkRecentChanges(inc, ins)

	return ins
}

// EnrichMassFailure fills in the root-cause sentence and recent-changes for a
// detected mass failure. The shared dependency is treated as the "incident"
// node so its transitive dependencies and change history explain why so many
// resources are failing at once.
func (e *Engine) EnrichMassFailure(mf MassFailure) MassFailure {
	parts := strings.SplitN(mf.SharedDependency, "/", 3)
	if len(parts) != 3 {
		return mf
	}

	if e.graph != nil {
		if cause, pattern := e.rootCauseOfRef(
			parts[0],
			parts[1],
			parts[2],
		); cause != "" {
			mf.RootCause = cause + fmt.Sprintf(" (pattern: %s)", pattern)
		}
	}

	if e.tracker != nil {
		recent := e.tracker.RecentChangesBefore(15 * time.Minute)
		depKey := parts[0] + "/" + parts[1] + "/" + parts[2]
		var changes []context.Change
		for _, c := range recent {
			if c.Resource+"/"+c.Namespace+"/"+c.Name == depKey &&
				c.Type == context.ChangeUpdate {
				changes = append(changes, c)
				if len(changes) >= 3 {
					break
				}
			}
		}
		mf.RecentChanges = changes
	}

	return mf
}

// rootCauseOfRef resolves the deepest dependencies of a resource key without
// needing a full incident struct.
func (e *Engine) rootCauseOfRef(kind, ns, name string) (string, string) {
	if e.graph == nil {
		return "", ""
	}
	roots := walkBackToRoots(e.graph, kind+"/"+ns+"/"+name)
	if len(roots) == 0 {
		return "", ""
	}
	sortRoots(roots)
	return describeRootCauses(roots)
}

// graphKeysForIncident resolves the graph node keys for an incident. Pod
// incidents are keyed by their owner while the graph stores real pod names,
// so the affected pod set (inc.Resources) must be used to reach the right
// nodes. Workload incidents store Name as "namespace/name".
func graphKeysForIncident(inc *model.Incident) []string {
	switch inc.Resource {
	case "pod":
		if len(inc.Resources) > 0 {
			keys := make([]string, 0, len(inc.Resources))
			for podName := range inc.Resources {
				keys = append(keys, "pod/"+inc.Namespace+"/"+podName)
			}
			return keys
		}
		if inc.Name != "" {
			return []string{"pod/" + inc.Namespace + "/" + inc.Name}
		}
	case "node":
		return []string{"node//" + inc.NodeName}
	default:
		name := strings.TrimPrefix(inc.Name, inc.Namespace+"/")
		return []string{inc.Resource + "/" + inc.Namespace + "/" + name}
	}
	return nil
}

// dependenciesFor unions the dependencies of all graph nodes belonging to the
// incident, deduplicating results.
// DependenciesFor returns the shared-dependency keys an incident touches in
// the resource graph. Exported so the correlation engine can ask "is this
// failure already covered by a mass-failure alert?" without owning a graph.
func DependenciesFor(
	graph *context.ResourceGraph,
	inc *model.Incident,
) []string {
	return dependenciesFor(graph, inc)
}

func dependenciesFor(
	graph *context.ResourceGraph,
	inc *model.Incident,
) []string {
	seen := make(map[string]bool)
	var deps []string
	for _, k := range graphKeysForIncident(inc) {
		parts := strings.SplitN(k, "/", 3)
		for _, d := range graph.DependenciesOf(parts[0], parts[1], parts[2]) {
			if !seen[d] {
				seen[d] = true
				deps = append(deps, d)
			}
		}
	}
	sort.Strings(deps)
	return deps
}
