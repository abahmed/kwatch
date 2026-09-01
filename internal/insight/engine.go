package insight

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/feature"
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
	plan     feature.Plan
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

// SetFeaturePlan supplies the immutable capability plan built by the
// composition root. A zero plan is kept backward compatible for package
// users that construct an insight engine directly.
func (e *Engine) SetFeaturePlan(plan feature.Plan) { e.plan = plan }

func (e *Engine) featureEnabled(id feature.ID) bool {
	if len(e.plan.Decisions) == 0 {
		return true
	}
	return e.plan.Enabled(id)
}

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

	if e.featureEnabled(feature.DirectDiagnosis) {
		e.determineCause(inc, ins)
	}
	if e.featureEnabled(feature.ImpactAnalysis) {
		e.describeImpact(inc, ins)
	}
	if e.featureEnabled(feature.ChangeDiff) {
		e.checkRecentChanges(inc, ins)
	}
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
	if e.featureEnabled(feature.DirectDiagnosis) {
		ins.NextSteps = nextSteps(inc)
	}
}

func (e *Engine) appendObservedEvidence(inc *model.Incident, ins *Insight) int {
	if inc.Events != "" {
		ins.Evidence = append(ins.Evidence, "Kubernetes warning events were observed")
	}
	if inc.Logs != "" {
		ins.Evidence = append(ins.Evidence, "container logs were collected")
	}
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
	if len(ins.RecentChanges) > 0 {
		ins.Evidence = append(ins.Evidence, "a related resource changed shortly before the incident")
	}
	evidenceBefore := len(ins.Evidence)
	if e.featureEnabled(feature.DependencyGraph) {
		e.appendActiveDependencyEvidence(inc, ins)
	}
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
	case "root_cause":
		ins.Confidence = 0.40
	}
}

func (e *Engine) applyInsightAdjustments(inc *model.Incident, ins *Insight, evidenceBefore int) {
	if e.featureEnabled(feature.RCAFeedback) {
		e.applyFeedbackBias(inc, ins)
	}
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
		parts := strings.SplitN(dependency, "/", 3)
		if len(parts) != 3 || !e.activeChecker(parts[0], parts[1], parts[2]) {
			continue
		}
		label := parts[0] + " " + parts[2]
		if parts[1] != "" {
			label = parts[0] + " " + parts[1] + "/" + parts[2]
		}
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
	name := strings.TrimPrefix(inc.Name, inc.Namespace+"/")
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
	case "persistentvolumeclaim":
		return []string{"kubectl describe pvc " + name + namespaceArg(inc.Namespace)}
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
	parts := strings.SplitN(mf.SharedDependency, "/", 3)
	if len(parts) != 3 {
		return mf
	}

	if e.featureEnabled(feature.DependencyGraph) && e.graph != nil {
		if cause, pattern := e.rootCauseOfRef(
			parts[0],
			parts[1],
			parts[2],
		); cause != "" {
			mf.RootCause = cause + fmt.Sprintf(" (pattern: %s)", pattern)
		}
	}

	if e.featureEnabled(feature.ChangeDiff) && e.tracker != nil {
		recent := e.tracker.RecentChangesBeforeAt(15*time.Minute, e.now())
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
