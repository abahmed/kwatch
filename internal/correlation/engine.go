package correlation

import (
	"fmt"
	"hash/crc32"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	appsv1lister "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
)

// IncidentKey derives a dedup key from an event, mirroring the exact
// normalisation
// chain inside Process. It returns the same key that Process would compute.
func IncidentKey(
	ev event.Event,
	owner string,
	cs *model.ContainerState,
) model.IncidentKey {
	r := normalizeReason(ev.Reason)
	// A crash-looping container reports different reasons across its cycle:
	// constant.ReasonError/constant.ReasonOOMKilled when it terminates,
	// constant.ReasonCrashLoopBackOff while backing off. Once it's established
	// as looping, fold them all into ONE canonical key so the key is stable
	// regardless of the container's momentary state. This makes the startup
	// baseline (captured in whatever state the container was in) match the live
	// alert (fired from a possibly-different state), and treats the loop as a
	// single incident.
	if cs != nil && cs.RestartCount > defaultCrashLoopHighFreqThreshold {
		switch r {
		case constant.ReasonError,
			constant.ReasonOOMKilled,
			constant.ReasonCrashLoopBackOff,
			constant.ReasonCrashLoopHighFreq:
			r = constant.ReasonCrashLoopHighFreq
		}
	}
	// Cross-namespace dedup: for ImagePullBackOff with global scope (rate
	// limits, timeouts, DNS, TLS errors), use the group key so the same
	// underlying issue
	// maps to a single incident regardless of namespace.
	if r == constant.ReasonImagePullBackOff ||
		r == constant.ReasonErrImagePull {
		scope := classifyImagePullScope(ev.Message)
		switch scope {
		case "rate_limit", "pull_qps", "timeout", "conn_refused",
			"net_unreachable", "dns", "tls":
			return GlobalKey(r, scope)
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
	// Something else is speaking for this incident. It resolves and expires
	// silently; ReleaseSuppressed clears the flag before asking again.
	if inc.SuppressedBy != "" {
		return model.ActionSkip
	}
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

const defaultBaselineTTL = 24 * time.Hour
const defaultCrashLoopHighFreqThreshold = 5
const DefaultMaxBaseline = 2000

type Engine struct {
	mu    sync.Mutex
	state map[model.IncidentKey]*model.Incident
	// ns → key → inc
	namespaceIndex map[string]map[model.IncidentKey]*model.Incident
	config         Config
	// baseline is the startup baseline: incident key (BuildKey string form) →
	// pod name → first-seen unix ts. The keys intentionally stay raw strings:
	// this map crosses the ConfigMap persistence layer (controller/state),
	// which treats it as an opaque serialized blob.
	baseline     map[string]map[string]int64
	deployLister appsv1lister.DeploymentLister
	ssLister     appsv1lister.StatefulSetLister
	dsLister     appsv1lister.DaemonSetLister
	// node name → has active incident
	activeNodeIncidents map[string]bool
	// key: namespace/podName
	lastContainerIndex map[string]*model.ContainerState
	serviceLister      corev1lister.ServiceLister
	// key → cooldown expiry; prevents resolve→recreate cycle
	cleanupCooldown map[model.IncidentKey]time.Time
	// computeGroupKey output → group buffer
	groupBuffers map[string]*pendingGroup
	// gk → batch resolve tracker
	groupResolveTrackers map[string]*groupResolveTracker
	// gk → last notification state
	groupFlushStates map[string]*groupFlushState
	// reason|namespace → owners seen this window
	fanOutWindows map[string]*ownerWindow
	auditLogger   *audit.AuditLogger
	// true when state has changed since last SnapshotAll
	dirty bool
	now   func() time.Time
	// synthetic incidents, persisted + restored across restarts
	massFailures map[model.IncidentKey]*model.Incident
	// loggedSkips remembers the last skip reason audited per incident key so a
	// steady-state suppression is recorded once instead of on every poll. A
	// baselined HPA alone produced ~11k identical lines a day, drowning the
	// signal in the log.
	loggedSkips map[model.IncidentKey]string
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
		state: make(map[model.IncidentKey]*model.Incident),
		namespaceIndex: make(
			map[string]map[model.IncidentKey]*model.Incident,
		),
		config:               cfg,
		activeNodeIncidents:  make(map[string]bool),
		lastContainerIndex:   make(map[string]*model.ContainerState),
		cleanupCooldown:      make(map[model.IncidentKey]time.Time),
		groupBuffers:         make(map[string]*pendingGroup),
		groupResolveTrackers: make(map[string]*groupResolveTracker),
		groupFlushStates:     make(map[string]*groupFlushState),
		fanOutWindows:        make(map[string]*ownerWindow),
		loggedSkips:          make(map[model.IncidentKey]string),
		massFailures:         make(map[model.IncidentKey]*model.Incident),
	}
	if e.now == nil {
		e.now = time.Now
	}
	if cfg.Baseline != nil {
		e.SetBaseline(cfg.Baseline)
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
		if _, err := strconv.Atoi(
			suffix,
		); err == nil &&
			knownRetryReasons[base] {
			if base == constant.ReasonErrImagePull {
				return constant.ReasonImagePullBackOff
			}
			return base
		}
	}
	return reason
}

// Process runs one observed event through the engine and announces whatever
// it decides. The returned incident is a copy and the action is the decision
// that was (or was not) announced; callers do not notify on their own — the
// engine already did, through the lifecycle hook, so the live path and every
// timer-driven path share one audit, one diagnosis and one delivery.
func (e *Engine) Process(
	ev event.Event,
	owner string,
	cs *model.ContainerState,
) (*model.Incident, model.IncidentAction) {
	e.mu.Lock()
	inc, action := e.processLocked(ev, owner, cs)
	if inc != nil {
		inc = inc.Clone()
	}
	e.mu.Unlock()

	e.emit(transition{inc, action})
	return inc, action
}

// processLocked is the decision half of Process. Every event walks the same
// five stages in the same order:
//
//  1. baseline     — pre-existing at startup: never news.
//  2. attribution  — a symptom of a known cause (node, shared dependency,
//     owning workload): recorded against it, not announced. See attribution.go.
//  3. cooldown     — recently resolved and still broken: silent until the
//     window ends, so a resolve never ping-pongs with a re-create.
//  4. identity     — which incident is this? dedup, silent revival, crash-loop
//     key fold, escalation.
//  5. announcement — should it speak now, wait for its group, or stay quiet
//     because nothing observable changed? See grouping.go and edgeAction.
//
// Caller must hold e.mu.
func (e *Engine) processLocked(
	ev event.Event,
	owner string,
	cs *model.ContainerState,
) (*model.Incident, model.IncidentAction) {
	e.dirty = true

	key := IncidentKey(ev, owner, cs)

	res := ev.Resource
	if res == "" {
		res = "pod"
	}

	// Node events feed the inhibition map before anything can gate them, so
	// a baselined node still explains the pods on it.
	e.trackNodeIncident(res, ev.NodeName, ev.Reason)

	// 1. baseline
	if e.skipByBaseline(res, key, ev) {
		return nil, model.ActionSkip
	}

	now := e.now()

	// 2. attribution
	if c := e.attribute(ev, owner, key, res); c.kind != causeNone {
		return e.recordSymptom(c, ev, owner, cs, key, res, now)
	}

	// 3. cooldown
	if e.skipByCooldown(key, ev) {
		return nil, model.ActionSkip
	}

	// 4. identity
	if inc, ok := e.state[key]; ok {
		// Already resolved — silently revive instead of re-creating.
		// Re-creating would emit a CREATE notification, causing a
		// resolved→CREATE→resolved flip-flop cycle. Silent revival
		// keeps the existing incident active and returns ActionUpdate.
		if inc.State == model.StateResolved ||
			inc.State == model.StatePendingResolve {
			return e.refreshIncident(inc, ev, cs, owner, now)
		}

		if e.config.EscalationEnabled && cs != nil &&
			e.escalateRestartCount(inc, ev, cs, now) {
			return inc, e.edgeAction(inc)
		}
		return e.refreshIncident(inc, ev, cs, owner, now)
	}

	// When RestartCount crosses the CrashLoopHighFrequency threshold, the
	// incident key changes from the raw reason (e.g. constant.ReasonError) to
	// constant.ReasonCrashLoopHighFreq. Rather than orphaning the old incident
	// (which silently dropped it without a RESOLVED and fired a duplicate
	// CREATE for the same ongoing crash loop), migrate the existing incident
	// to the folded key: same ID (alert thread continuity), same FirstSeen,
	// no notification churn. Baseline entries are carried over so a startup-
	// baselined loop stays suppressed after the fold.
	if cs != nil && int(cs.RestartCount) > defaultCrashLoopHighFreqThreshold {
		if orphan, ok := e.foldCrashLoopIncident(ev, owner, key, res); ok {
			return e.refreshIncident(orphan, ev, cs, owner, now)
		}
	}

	inc := e.newIncident(ev, owner, cs, key, res, now)
	e.state[key] = inc
	// The key is alerting again, so a later suppression is newsworthy.
	e.clearSkipAudit(key)
	e.indexIncidentByNamespace(inc)

	// 5. announcement: buffer into a group, or speak on the edge.
	if e.tryGroupIncident(inc, ev, owner, now) {
		return inc, model.ActionSkip
	}

	return inc, e.edgeAction(inc)
}

// Caller must hold e.mu.
func (e *Engine) newIncident(
	ev event.Event,
	owner string,
	cs *model.ContainerState,
	key model.IncidentKey,
	res string,
	now time.Time,
) *model.Incident {
	inc := &model.Incident{
		Subject: model.Subject{
			ID:        fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(key))),
			Key:       key,
			Reason:    ev.Reason,
			Namespace: ev.Namespace,
			Resource:  res,
			Name:      owner,
			NodeName:  ev.NodeName,
		},
		Status: model.Status{
			Count:      1,
			FirstSeen:  now,
			LastSeen:   now,
			LastUpdate: now,
			State:      model.StateActive,
			Resources:  map[string]bool{},
			Containers: map[string]bool{},
		},
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
			// seed from the absolute threshold when no tier is crossed at
			// startup
			for i := len(e.config.EscalationTiers) - 1; i >= 0; i-- {
				if cur >= e.config.EscalationTiers[i] {
					ev.Severity = severityForTier(i, inc.Severity)
					break
				}
			}
		}
	}
	e.config.Enricher.Enrich(&ev, inc)

	// Topology is impact, not explanation. It used to be appended to the
	// hint as prose, where it repeated what the diagnosis block already says
	// and made the hint unreadable. Keep it structured; renderers decide how
	// to show it.
	if deps := e.findDependentServices(ev.Namespace, ev.Labels); len(deps) > 0 {
		sort.Strings(deps)
		inc.AffectedServices = deps
	}
	if res == "pod" && owner != "" && !e.isOwnerHealthy(inc) {
		inc.OwnerUnhealthy = true
	}

	return inc
}
