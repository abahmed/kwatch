package correlation

import (
	"fmt"
	"hash/crc32"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
)

// BuildKey constructs the incident key used for dedup, grouping, and baseline.
func BuildKey(namespace, owner, reason, container string) string {
	return namespace + ":" + owner + ":" + reason + ":" + container
}

// IncidentKey derives a dedup key from an event, mirroring the exact normalisation
// chain inside Process. It returns the same key that Process would compute.
func IncidentKey(ev event.Event, owner string, cs *model.ContainerState) string {
	r := normalizeReason(ev.Reason)
	// A crash-looping container reports different reasons across its cycle:
	// constant.ReasonError/constant.ReasonOOMKilled when it terminates, constant.ReasonCrashLoopBackOff while backing off.
	// Once it's established as looping, fold them all into ONE canonical key so the key
	// is stable regardless of the container's momentary state. This makes the startup
	// baseline (captured in whatever state the container was in) match the live alert
	// (fired from a possibly-different state), and treats the loop as a single incident.
	if cs != nil && cs.RestartCount > defaultCrashLoopHighFreqThreshold {
		switch r {
		case constant.ReasonError, constant.ReasonOOMKilled, constant.ReasonCrashLoopBackOff, constant.ReasonCrashLoopHighFreq:
			r = constant.ReasonCrashLoopHighFreq
		}
	}
	// Cross-namespace dedup: for ImagePullBackOff with global scope (rate limits,
	// timeouts, DNS, TLS errors), use the group key so the same underlying issue
	// maps to a single incident regardless of namespace.
	if r == constant.ReasonImagePullBackOff || r == constant.ReasonErrImagePull {
		scope := classifyImagePullScope(ev.Message)
		switch scope {
		case "rate_limit", "pull_qps", "timeout", "conn_refused",
			"net_unreachable", "dns", "tls":
			return r + "|global|" + scope
		}
	}
	return BuildKey(ev.Namespace, owner, r, "")
}

func notifSig(inc *model.Incident) string {
	st := "firing"
	if inc.State == model.StateResolved {
		st = "resolved"
	}
	return st + "|" + string(inc.Severity)
}

// edgeAction returns the action to notify, or ActionSkip if nothing changed.
func (e *Engine) edgeAction(inc *model.Incident) model.IncidentAction {
	sig := notifSig(inc)
	if sig == inc.NotifiedSig {
		return model.ActionSkip
	}
	prev := inc.NotifiedSig
	inc.NotifiedSig = sig
	inc.LastNotifiedAt = e.now()
	if inc.State == model.StateResolved {
		metrics.Default.IncidentsResolved.Add(1)
		return model.ActionResolved
	}
	if prev == "" {
		metrics.Default.IncidentsCreate.Add(1)
		return model.ActionCreate
	}
	metrics.Default.IncidentsUpdate.Add(1)
	return model.ActionUpdate
}

// crossedTier returns the highest index of a tier whose threshold was
// crossed when moving from prev to new restarts, or -1.
func crossedTier(prev, new int, tiers []int) int {
	hit := -1
	for i, t := range tiers {
		if prev < t && new >= t {
			hit = i
		}
	}
	return hit
}

// severityForTier returns the severity for the given escalation tier index,
// preferring the higher of the tier-based severity and the current severity.
func severityForTier(tierIdx int, current model.Severity) model.Severity {
	sev := model.SeverityCritical
	switch tierIdx {
	case 0:
		sev = model.SeverityHigh
	}
	if current.Rank() > sev.Rank() {
		return current
	}
	return sev
}

const defaultBaselineTTL = 24 * time.Hour
const defaultCrashLoopHighFreqThreshold = 5
const DefaultMaxBaseline = 2000

type Engine struct {
	mu                  sync.Mutex
	state               map[string]*model.Incident
	namespaceIndex      map[string]map[string]*model.Incident // ns → key → inc
	config              Config
	seen                map[string]map[string]int64
	deployLister        appsv1lister.DeploymentLister
	ssLister            appsv1lister.StatefulSetLister
	dsLister            appsv1lister.DaemonSetLister
	activeNodeIncidents map[string]bool
	lastContainerIndex  map[string]*model.ContainerState // key: namespace/podName
	serviceLister       corev1lister.ServiceLister
	cleanupCooldown     map[string]time.Time            // key → cooldown expiry; prevents resolve→recreate cycle
	pendingGroups       map[string]*pendingGroup        // computeGroupKey output → group buffer
	groupMembers        map[string]*groupResolveTracker // gk → batch resolve tracker
	groupFlush          map[string]*groupFlushState     // gk → last notification state
	deferredResolves    []*model.Incident               // group resolves awaiting the next lifecycle tick
	auditLogger         *audit.AuditLogger
	dirty               bool // true when state has changed since last SnapshotAll
	now                 func() time.Time
}

func NewEngine(cfg Config) *Engine {
	if cfg.Enricher == nil {
		cfg.Enricher = &enricher.DefaultEnricher{}
	}
	if cfg.LifecycleInterval <= 0 {
		cfg.LifecycleInterval = 1 * time.Minute
	}
	if cfg.BaselineTTL <= 0 {
		cfg.BaselineTTL = defaultBaselineTTL
	}
	if cfg.MaxBaseline <= 0 {
		cfg.MaxBaseline = DefaultMaxBaseline
	}
	e := &Engine{
		state:               make(map[string]*model.Incident),
		namespaceIndex:      make(map[string]map[string]*model.Incident),
		config:              cfg,
		activeNodeIncidents: make(map[string]bool),
		lastContainerIndex:  make(map[string]*model.ContainerState),
		cleanupCooldown:     make(map[string]time.Time),
		pendingGroups:       make(map[string]*pendingGroup),
		groupMembers:        make(map[string]*groupResolveTracker),
		groupFlush:          make(map[string]*groupFlushState),
	}
	if e.now == nil {
		e.now = time.Now
	}
	if cfg.Baseline != nil {
		e.SetSeen(cfg.Baseline)
	}
	return e
}

var knownRetryReasons = map[string]bool{
	constant.ReasonCrashLoopBackOff: true,
	constant.ReasonBackOff:          true,
	constant.ReasonErrImagePull:     true,
	constant.ReasonImagePullBackOff: true,
}

func normalizeReason(reason string) string {
	if reason == constant.ReasonErrImagePull {
		return constant.ReasonImagePullBackOff
	}
	idx := strings.LastIndex(reason, " ")
	if idx > 0 {
		base, suffix := reason[:idx], reason[idx+1:]
		if _, err := strconv.Atoi(suffix); err == nil && knownRetryReasons[base] {
			if base == constant.ReasonErrImagePull {
				return constant.ReasonImagePullBackOff
			}
			return base
		}
	}
	return reason
}

// findDependentServices returns the names of Services in the given namespace
// whose selectors match the provided pod labels. Returns nil if no service
// lister is configured or no matches are found.
func (e *Engine) findDependentServices(namespace string, podLabels map[string]string) []string {
	if e.serviceLister == nil || len(podLabels) == 0 {
		return nil
	}
	svcs, err := e.serviceLister.Services(namespace).List(labels.Everything())
	if err != nil {
		return nil
	}
	var result []string
	for _, svc := range svcs {
		if len(svc.Spec.Selector) == 0 {
			continue
		}
		match := true
		for k, v := range svc.Spec.Selector {
			if podLabels[k] != v {
				match = false
				break
			}
		}
		if match {
			result = append(result, svc.Name)
		}
	}
	return result
}

func (e *Engine) isOwnerHealthy(inc *model.Incident) bool {
	if inc.Resource != "pod" {
		return true
	}
	ns := inc.Namespace
	name := inc.Name
	if ns == "" || name == "" {
		return true
	}

	switch inc.OwnerKind {
	case "Deployment":
		if e.deployLister == nil {
			return true
		}
		d, err := e.deployLister.Deployments(ns).Get(name)
		if err != nil {
			return len(inc.Resources) == 0
		}
		if d.Status.ObservedGeneration < d.Generation {
			return false
		}
		return d.Status.ReadyReplicas >= d.Status.Replicas &&
			d.Status.UnavailableReplicas == 0

	case "StatefulSet":
		if e.ssLister == nil {
			return true
		}
		ss, err := e.ssLister.StatefulSets(ns).Get(name)
		if err != nil {
			return len(inc.Resources) == 0
		}
		if ss.Status.ObservedGeneration < ss.Generation {
			return false
		}
		return ss.Status.ReadyReplicas >= ss.Status.Replicas &&
			ss.Status.CurrentRevision == ss.Status.UpdateRevision

	case "DaemonSet":
		if e.dsLister == nil {
			return true
		}
		ds, err := e.dsLister.DaemonSets(ns).Get(name)
		if err != nil {
			return len(inc.Resources) == 0
		}
		return ds.Status.DesiredNumberScheduled > 0 &&
			ds.Status.NumberUnavailable == 0 &&
			ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled

	default:
		return true
	}
}

func (e *Engine) Process(ev event.Event, owner string, cs *model.ContainerState) (incident *model.Incident, action model.IncidentAction) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dirty = true
	defer func() {
		if incident != nil {
			incident = incident.Clone()
		}
	}()

	key := IncidentKey(ev, owner, cs)

	res := ev.Resource
	if res == "" {
		res = "pod"
	}

	// Track active node incidents for pod suppression — must happen before
	// the baseline check so node events always populate the inhibition map.
	e.trackNodeIncident(res, ev.NodeName, ev.Reason)

	// Baseline — skip for node events so the incident is always created
	if res != "node" && e.isBaselined(key, ev.PodName) {
		if e.auditLogger != nil {
			e.auditLogger.LogSkip(&model.Incident{Key: key, Namespace: ev.Namespace, Reason: ev.Reason, ID: key}, "baseline")
		}
		return nil, model.ActionSkip
	}

	// Suppress pod incidents when the node has an active incident
	if e.config.InhibitNodeSuppressesPods && res == "pod" {
		if e.suppressedByNodeIncident(ev, owner, key) {
			return nil, model.ActionSkip
		}
	}

	// Cooldown check — suppress re-creation after cleanup for still-broken resources
	if expiry, ok := e.cleanupCooldown[key]; ok {
		if e.now().Before(expiry) {
			if e.auditLogger != nil {
				e.auditLogger.LogSkip(&model.Incident{Key: key, Namespace: ev.Namespace, Reason: ev.Reason, ID: key}, "cooldown")
			}
			return nil, model.ActionSkip
		}
		delete(e.cleanupCooldown, key)
	}

	now := e.now()

	// Cascading suppression: if a pod incident fires and its owning workload
	// already has an active (non-pod) incident, suppress the pod as a symptom.
	if res == "pod" && owner != "" {
		prefix := ev.Namespace + ":" + owner + ":"
		for _, existing := range e.state {
			if existing.State == model.StateResolved ||
				existing.State == model.StatePendingResolve {
				continue
			}
			if existing.Resource != "pod" &&
				existing.Namespace == ev.Namespace &&
				existing.Name == owner &&
				strings.HasPrefix(existing.Key, prefix) {
				existing.Count++
				if ev.PodName != "" {
					existing.Resources[ev.PodName] = true
					if len(existing.Resources) > existing.PeakResources {
						existing.PeakResources = len(existing.Resources)
					}
				}
				existing.LastSeen = now
				if e.auditLogger != nil {
					e.auditLogger.LogSkip(&model.Incident{Key: key, Namespace: ev.Namespace, Reason: ev.Reason, ID: key}, "cascading_suppression")
				}
				return nil, model.ActionSkip
			}
		}
	}

	if inc, ok := e.state[key]; ok {
		// Already resolved — silently revive instead of re-creating.
		// Re-creating would emit a CREATE notification, causing a
		// resolved→CREATE→resolved flip-flop cycle. Silent revival
		// keeps the existing incident active and returns ActionUpdate.
		if inc.State == model.StateResolved {
			return e.refreshIncident(inc, ev, cs, owner, now)
		}

		// Pending resolve — revoke the scheduled resolve
		if inc.State == model.StatePendingResolve {
			return e.refreshIncident(inc, ev, cs, owner, now)
		}

		if e.config.EscalationEnabled && cs != nil {
			prev := inc.RestartCount
			cur := int(cs.RestartCount)
			if t := crossedTier(prev, cur, e.config.EscalationTiers); t >= 0 {
				ev.Severity = severityForTier(t, inc.Severity)
				e.config.Enricher.Enrich(&ev, inc)
				inc.Hint = fmt.Sprintf("restart count crossed %d", e.config.EscalationTiers[t])
				inc.Count++
				inc.LastSeen = now
				inc.State = model.StateActive
				inc.LastUpdate = now
				inc.RestartCount = cur
				if ev.PodName != "" {
					inc.Resources[ev.PodName] = true
					if len(inc.Resources) > inc.PeakResources {
						inc.PeakResources = len(inc.Resources)
					}
				}
				if ev.ContainerName != "" && ev.ContainerName != "." {
					inc.Containers[ev.ContainerName] = true
				}
				inc.LastContainerState = cs
				e.indexLastContainerState(ev.Namespace, ev.PodName, ev.ContainerName, cs)
				return inc, e.edgeAction(inc)
			}
		}
		return e.refreshIncident(inc, ev, cs, owner, now)
	}

	// When RestartCount crosses the CrashLoopHighFrequency threshold, the
	// incident key changes from the raw reason (e.g. constant.ReasonError) to
	// constant.ReasonCrashLoopHighFreq, orphaning the old incident in the state map.
	// Silently resolve any orphaned incidents with the same ns:owner: prefix
	// so the state map doesn't accumulate stale entries for the same pod.
	if cs != nil && int(cs.RestartCount) > defaultCrashLoopHighFreqThreshold {
		prefix := ev.Namespace + ":" + owner + ":"
		for k, oldInc := range e.state {
			if strings.HasPrefix(k, prefix) && k != key &&
				oldInc.State != model.StateResolved &&
				oldInc.State != model.StatePendingResolve &&
				oldInc.Resource == res {
				oldInc.State = model.StateResolved
				e.removeIncidentFromNamespaceIndex(oldInc)
				delete(e.state, k)
				delete(e.seen, k)
				// Release the folded incident from any smart-group tracker so
				// the group isn't stuck waiting on a member that no longer
				// exists. If that resolves the whole group, defer the RESOLVED
				// notification to the next lifecycle tick (Process holds the
				// lock, so hooks cannot run here).
				if groupInc, groupAction, tracked := e.groupMemberResolved(k); tracked && groupAction == model.ActionResolved {
					e.deferredResolves = append(e.deferredResolves, groupInc)
				}
			}
		}
	}

	inc := e.newIncident(ev, owner, cs, key, res, now)
	e.state[key] = inc
	e.indexIncidentByNamespace(inc)

	// Smart grouping: buffer same-reason incidents, suppress individual
	// notification until the group window expires.
	if e.tryGroupIncident(inc, ev, owner, now) {
		return inc, model.ActionSkip
	}

	return inc, e.edgeAction(inc)
}

// Caller must hold e.mu.
func (e *Engine) newIncident(ev event.Event, owner string, cs *model.ContainerState, key, res string, now time.Time) *model.Incident {
	inc := &model.Incident{
		ID:         fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(key))),
		Key:        key,
		Reason:     ev.Reason,
		Namespace:  ev.Namespace,
		Resource:   res,
		Name:       owner,
		NodeName:   ev.NodeName,
		Count:      1,
		FirstSeen:  now,
		LastSeen:   now,
		LastUpdate: now,
		State:      model.StateActive,
		Resources:  map[string]bool{},
		Containers: map[string]bool{},
	}
	if ev.PodName != "" {
		inc.Resources[ev.PodName] = true
	}
	inc.PeakResources = len(inc.Resources)
	if ev.ContainerName != "" && ev.ContainerName != "." {
		inc.Containers[ev.ContainerName] = true
	}
	// Bare pods (no owner) are identified by their own name.
	if owner == "" {
		inc.Name = ev.PodName
	}
	inc.LastContainerState = cs
	e.indexLastContainerState(ev.Namespace, ev.PodName, ev.ContainerName, cs)
	if cs != nil {
		inc.RestartCount = int(cs.RestartCount)
	}
	if url, ok := e.config.Runbooks[ev.Reason]; ok {
		inc.Runbook = url
	}
	if e.config.EscalationEnabled && cs != nil {
		cur := int(cs.RestartCount)
		if t := crossedTier(-1, cur, e.config.EscalationTiers); t >= 0 {
			ev.Severity = severityForTier(t, inc.Severity)
		} else if ev.Severity == "" {
			// seed from the absolute threshold when no tier is crossed at startup
			for i := len(e.config.EscalationTiers) - 1; i >= 0; i-- {
				if cur >= e.config.EscalationTiers[i] {
					ev.Severity = severityForTier(i, inc.Severity)
					break
				}
			}
		}
	}
	e.config.Enricher.Enrich(&ev, inc)

	// Topological annotation: dependent services
	if deps := e.findDependentServices(ev.Namespace, ev.Labels); len(deps) > 0 {
		inc.Hint = enricher.CombineHints(inc.Hint,
			fmt.Sprintf("affects service(s): %s", strings.Join(deps, ", ")))
	}

	// Topological annotation: parent workload health
	if res == "pod" && owner != "" && !e.isOwnerHealthy(inc) {
		inc.Hint = enricher.CombineHints(inc.Hint,
			fmt.Sprintf("owning %s %s is also unhealthy", ev.OwnerKind, owner))
	}

	return inc
}
