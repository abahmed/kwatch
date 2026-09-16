package insight

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	context "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/model"
)

type Insight struct {
	Cause         string
	Impact        string
	Pattern       string
	Confidence    float64
	Evidence      []string
	NextSteps     []string
	AffectedCount int
	RecentChanges []context.Change
}

type Engine struct {
	graph   *context.ResourceGraph
	tracker *context.ChangeTracker
	// activeChecker lets impact analysis distinguish live affected resources
	// from merely declared graph dependents. It is optional for standalone use.
	activeChecker func(kind, namespace, name string) bool
	// now is the clock "updated 3m ago" is measured against; injectable so
	// tests do not depend on the wall clock.
	now      func() time.Time
	feedback *FeedbackStore
}

// Dependencies contains the optional application-owned collaborators used by
// insight analysis. Keeping them together makes construction complete before
// the engine is exposed to incident processing.
type Dependencies struct {
	Clock         clock.Clock
	FeedbackStore *FeedbackStore
	ActiveChecker func(kind, namespace, name string) bool
}

func (e *Engine) ObserveOutcome(inc *model.Incident, action model.IncidentAction, pattern string) {
	if e.feedback != nil {
		e.feedback.Observe(inc, action, pattern)
	}
}

// NewEngineWithClock constructs insight analysis with an explicit clock.
func NewEngineWithClock(
	graph *context.ResourceGraph,
	tracker *context.ChangeTracker,
	timeSource clock.Clock,
) *Engine {
	timeSource = clock.Require(timeSource)
	return &Engine{
		graph: graph, tracker: tracker, now: timeSource.Now,
	}
}

// NewEngineWithDependencies constructs insight with all runtime collaborators
// supplied at the composition root.
func NewEngineWithDependencies(
	graph *context.ResourceGraph,
	tracker *context.ChangeTracker,
	dependencies Dependencies,
) *Engine {
	dependencies.Clock = clock.Require(dependencies.Clock)
	return &Engine{
		graph:         graph,
		tracker:       tracker,
		now:           dependencies.Clock.Now,
		feedback:      dependencies.FeedbackStore,
		activeChecker: dependencies.ActiveChecker,
	}
}

func (e *Engine) Analyze(inc *model.Incident) *Insight {
	ins := &Insight{}

	e.determineCause(inc, ins)
	e.describeImpact(inc, ins)
	e.checkRecentChanges(inc, ins)
	e.scoreInsight(inc, ins)

	return ins
}

// EnrichMassFailure fills in the root-cause sentence and recent-changes for a
// detected mass failure. The shared dependency is treated as the "incident"
// node so its transitive dependencies and change history explain why so many
// resources are failing at once.
func (e *Engine) EnrichMassFailure(mf MassFailure) MassFailure {
	ref, ok := model.ParseObjectKey(mf.SharedDependency)
	if !ok {
		return mf
	}

	if e.graph != nil {
		if cause, pattern := e.rootCauseOfRef(
			ref.Kind,
			ref.Namespace,
			ref.Name,
		); cause != "" {
			mf.RootCause = cause + fmt.Sprintf(" (pattern: %s)", pattern)
		}
	}

	if e.tracker != nil {
		recent := e.tracker.RecentChangesBeforeAt(15*time.Minute, e.now())
		depKey := ref.Key()
		var changes []context.Change
		for _, c := range recent {
			if model.ObjectKey(c.Resource, c.Namespace, c.Name) == depKey &&
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

// graphKeysForIncident resolves the graph node keys for an incident. The
// mapping from an incident to the concrete objects it is about -- Pod
// incidents are keyed by their owner while the graph stores real Pod names --
// belongs to model.ObjectRefs, so this is only the key rendering.
func graphKeysForIncident(inc *model.Incident) []string {
	refs := inc.ObjectRefs()
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, ref.Key())
	}
	return keys
}

// IncidentGraphKeys exposes the normalized graph identities used by insight
// so other composition-layer components can match active incidents without
// duplicating the pod/owner naming rules.
func IncidentGraphKeys(inc *model.Incident) []string {
	if inc == nil {
		return nil
	}
	return graphKeysForIncident(inc)
}

// dependenciesFor unions the dependencies of all graph nodes belonging to the
// incident, deduplicating results.
// DependenciesFor returns the shared-dependency keys an incident touches in
// the resource graph. Exported so the incident engine can ask "is this
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
