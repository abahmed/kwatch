package insight

import (
	"fmt"
	"sort"
	"strings"
	"time"

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

// SetActiveChecker supplies the correlation engine's live incident view. The
// callback is deliberately narrow so insight does not depend on correlation.
func (e *Engine) SetActiveChecker(checker func(kind, namespace, name string) bool) {
	e.activeChecker = checker
}

// SetClock injects the clock used for recent-change analysis.
func (e *Engine) SetClock(now func() time.Time) {
	if now != nil {
		e.now = now
	}
}

func (e *Engine) SetFeedbackStore(store *FeedbackStore) { e.feedback = store }

func (e *Engine) ObserveOutcome(inc *model.Incident, action model.IncidentAction, pattern string) {
	if e.feedback != nil {
		e.feedback.Observe(inc, action, pattern)
	}
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
	e.scoreInsight(inc, ins)

	return ins
}

// scoreInsight turns topology and observed signals into an explainable
// confidence value. A graph relationship alone is intentionally weak evidence;
// a matching node/workload failure, event, or recent change raises confidence.
func (e *Engine) scoreInsight(inc *model.Incident, ins *Insight) {
	if inc == nil {
		return
	}
	evidenceBefore := e.appendObservedEvidence(inc, ins)
	e.setPatternConfidence(ins)
	e.applyInsightAdjustments(inc, ins, evidenceBefore)
	ins.NextSteps = nextSteps(inc)
}

func (e *Engine) appendObservedEvidence(inc *model.Incident, ins *Insight) int {
	if inc.OwnerUnhealthy {
		ins.Evidence = append(ins.Evidence, "the owning workload is unhealthy")
	}
	if inc.Facts.MemoryLeak {
		ins.Evidence = append(ins.Evidence, fmt.Sprintf(
			"repeated OOM kills were observed in a %d-minute window",
			inc.Facts.OOMWindowMin,
		))
	}
	if inc.Facts.ProbeEndpoint != "" {
		ins.Evidence = append(ins.Evidence, "probe failed: "+inc.Facts.ProbeEndpoint)
	}
	if inc.Facts.SchedulingDelay > 0 {
		ins.Evidence = append(ins.Evidence, fmt.Sprintf(
			"the workload has remained unscheduled for %s",
			inc.Facts.SchedulingDelay.Round(time.Second),
		))
	}
	if inc.Facts.PullSecretsSet {
		ins.Evidence = append(ins.Evidence, "the pod declares image pull secrets")
	}
	if inc.Facts.Volume != "" {
		ins.Evidence = append(
			ins.Evidence, "bound volume: "+inc.Facts.Volume,
		)
	}
	if len(ins.RecentChanges) > 0 {
		ins.Evidence = append(ins.Evidence, "a related resource changed shortly before the incident")
	}
	evidenceBefore := len(ins.Evidence)
	e.appendActiveDependencyEvidence(inc, ins)
	return evidenceBefore
}

func (e *Engine) setPatternConfidence(ins *Insight) {
	switch ins.Pattern {
	case "node_failure":
		ins.Confidence = 0.90
	case "rollout_failure", "storage_failure", "storage_attachment_failure":
		ins.Confidence = 0.85
	case "dependency_change", "config_error":
		ins.Confidence = 0.60
	case "resource_limit":
		// The reported metric is the cause by definition.
		ins.Confidence = 0.85
	case "node_pressure":
		ins.Confidence = 0.80
	case "metrics_unavailable":
		ins.Confidence = 0.70
	case "root_cause":
		ins.Confidence = 0.40
	}
}

func (e *Engine) applyInsightAdjustments(inc *model.Incident, ins *Insight, evidenceBefore int) {
	e.applyFeedbackBias(inc, ins)
	if ins.Confidence > 0 && len(ins.Evidence) == 0 {
		ins.Confidence *= 0.65
	}
	if ins.Confidence > 0 && evidenceBefore == 0 && len(ins.Evidence) > 0 {
		ins.Confidence = minFloat(ins.Confidence+0.10, 1)
	}
}

func (e *Engine) appendActiveDependencyEvidence(
	inc *model.Incident,
	ins *Insight,
) {
	if e.activeChecker == nil || e.graph == nil {
		return
	}
	for _, dependency := range dependenciesFor(e.graph, inc) {
		ref, ok := model.ParseObjectKey(dependency)
		if !ok || !e.activeChecker(ref.Kind, ref.Namespace, ref.Name) {
			continue
		}
		label := ref.Describe()
		ins.Evidence = append(ins.Evidence, "an active incident is already reported for "+label)
		return
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func (e *Engine) applyFeedbackBias(inc *model.Incident, ins *Insight) {
	if e.feedback == nil || ins.Pattern == "" {
		return
	}
	ins.Confidence = minFloat(ins.Confidence+e.feedback.Bias(feedbackKey(inc, ins.Pattern)), 1)
	if ins.Confidence < 0 {
		ins.Confidence = 0
	}
}

func nextSteps(inc *model.Incident) []string {
	if inc == nil {
		return nil
	}
	name := inc.Ref().Name
	if inc.Resource == "pod" && len(inc.Resources) > 0 {
		pods := make([]string, 0, len(inc.Resources))
		for pod := range inc.Resources {
			pods = append(pods, pod)
		}
		sort.Strings(pods)
		name = pods[0]
	}
	switch inc.Resource {
	case "pod":
		return []string{"kubectl describe pod " + name + namespaceArg(inc.Namespace), "kubectl logs " + name + namespaceArg(inc.Namespace) + " --all-containers"}
	case "node":
		return []string{"kubectl describe node " + name, "kubectl get pods -A --field-selector spec.nodeName=" + name}
	case "deployment":
		return []string{"kubectl rollout status deployment/" + name + namespaceArg(inc.Namespace), "kubectl rollout history deployment/" + name + namespaceArg(inc.Namespace)}
	case "pvc", "persistentvolumeclaim":
		// Storage incidents carry the resource kind "pvc", which is the
		// vocabulary the graph and the PVC monitor use; the longer spelling
		// never matched an incident, so PVC alerts arrived with no next step.
		return []string{
			"kubectl describe pvc " + name + namespaceArg(inc.Namespace),
			"kubectl get pv" + namespaceArg(inc.Namespace),
		}
	default:
		return nil
	}
}

func namespaceArg(namespace string) string {
	if namespace == "" {
		return ""
	}
	return " -n " + namespace
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
