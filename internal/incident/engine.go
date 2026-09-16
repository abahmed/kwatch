package incident

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
)

// IncidentKey derives a dedup key from an event, mirroring the exact
// normalisation
// chain inside Process. It returns the same key that Process would compute.
// ObservationKey is the incident key an observation will be folded into. It
// is exported for the startup baseline, which has to record exactly the key
// the live path will produce, and for monitors that keep their own
// per-incident hysteresis state.
func ObservationKey(obs *model.Observation) model.IncidentKey {
	if obs == nil {
		return ""
	}
	return IncidentKey(
		event.FromObservation(obs), obs.OwnerPath(), obs.State(),
	)
}

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
	return BuildKey(ev.Namespace, incidentOwner(ev, owner), r, "")
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
		metrics.DefaultRegistry().IncidentsResolved.Add(1)
		return model.ActionResolved
	}
	if prev == "" {
		metrics.DefaultRegistry().IncidentsCreate.Add(1)
		return model.ActionCreate
	}
	metrics.DefaultRegistry().IncidentsUpdate.Add(1)
	return model.ActionUpdate
}

const defaultBaselineTTL = 24 * time.Hour

// defaultOwnerBaselineTTL is the longest detector sustain window plus a
// margin: long enough that a rollout in progress at startup is not
// announced, short enough that it cannot hide a later real failure.
const defaultOwnerBaselineTTL = 10 * time.Minute
const defaultCrashLoopHighFreqThreshold = 5

// DefaultMaxBaseline mirrors config.DefaultConfig().Correlation.MaxBaseline
// so the fallback and the shipped default are the same number.
const DefaultMaxBaseline = 5000

// incidentIndexes groups the lookup tables that must change together with
// the active incident state.
type incidentIndexes struct {
	namespaceIndex map[string]map[model.IncidentKey]*model.Incident
	subjectIndex   map[model.ObjectRef]map[model.IncidentKey]struct{}
	nodeIncidents  map[string]map[model.IncidentKey]struct{}
}

// lifecycleState contains active incidents and the indexes used to reconcile
// them. It is embedded so the engine's single-mutex invariants stay obvious at
// call sites while ownership remains explicit here.
type lifecycleState struct {
	state map[model.IncidentKey]*model.Incident
	incidentIndexes
	lastContainerIndex map[string]containerStateEntry
	podResourceUIDs    map[model.IncidentKey]map[string]string
	cleanupCooldown    map[model.IncidentKey]time.Time
	massFailures       map[model.IncidentKey]*model.Incident
	loggedSkips        map[model.IncidentKey]string
	reconciler         *observationReconciler
}

// baselineState contains startup suppression and node-inhibition state.
type baselineState struct {
	baseline            map[string]map[string]int64
	baselineByOwner     map[string]map[string]struct{}
	activeNodeIncidents map[string]bool
}

// groupingState contains only mutable smart-group windows and trackers.
type groupingState struct {
	groupBuffers         map[string]*pendingGroup
	groupResolveTrackers map[string]*groupResolveTracker
	groupFlushStates     map[string]*groupFlushState
	fanOutWindows        map[string]*ownerWindow
}

type Engine struct {
	mu sync.Mutex
	lifecycleState
	baselineState
	groupingState
	// frozen is set immediately before the final shutdown snapshot. Dynamic
	// informer callbacks may still be unwinding after their stop channel
	// closes, so runtime incident mutations must become no-ops at that point.
	frozen                bool
	config                Config
	attributionSources    AttributionSources
	attributionConfigured bool
	processingStarted     bool
	auditLogger           SkipLogger
	// true when state has changed since last SnapshotAll
	dirty bool
	now   func() time.Time
}

// NewEngineWithClock constructs an incident engine with an explicit clock.
// Application composition must use this constructor so time-based decisions
// do not depend on package-level wall-clock state.
func NewEngineWithClock(cfg Config, runtimeClock clock.Clock) *Engine {
	runtimeClock = clock.Require(runtimeClock)
	cfg.Now = runtimeClock.Now
	return newEngine(cfg)
}

func newEngine(cfg Config) *Engine {
	if cfg.Enricher == nil {
		cfg.Enricher = &enricher.DefaultEnricher{}
	}
	if cfg.LifecycleInterval <= 0 {
		cfg.LifecycleInterval = 1 * time.Minute
	}
	if cfg.BaselineTTL <= 0 {
		cfg.BaselineTTL = defaultBaselineTTL
	}
	if cfg.OwnerBaselineTTL <= 0 {
		cfg.OwnerBaselineTTL = defaultOwnerBaselineTTL
	}
	if cfg.MaxBaseline <= 0 {
		cfg.MaxBaseline = DefaultMaxBaseline
	}
	e := &Engine{
		lifecycleState: lifecycleState{
			state: make(map[model.IncidentKey]*model.Incident),
			incidentIndexes: incidentIndexes{
				namespaceIndex: make(
					map[string]map[model.IncidentKey]*model.Incident,
				),
				subjectIndex: make(
					map[model.ObjectRef]map[model.IncidentKey]struct{},
				),
				nodeIncidents: make(
					map[string]map[model.IncidentKey]struct{},
				),
			},
			lastContainerIndex: make(map[string]containerStateEntry),
			podResourceUIDs: make(
				map[model.IncidentKey]map[string]string,
			),
			cleanupCooldown: make(map[model.IncidentKey]time.Time),
			massFailures:    make(map[model.IncidentKey]*model.Incident),
			loggedSkips:     make(map[model.IncidentKey]string),
			reconciler:      newObservationReconciler(),
		},
		baselineState: baselineState{
			baseline:            make(map[string]map[string]int64),
			baselineByOwner:     make(map[string]map[string]struct{}),
			activeNodeIncidents: make(map[string]bool),
		},
		groupingState: groupingState{
			groupBuffers: make(map[string]*pendingGroup),
			groupResolveTrackers: make(
				map[string]*groupResolveTracker,
			),
			groupFlushStates: make(map[string]*groupFlushState),
			fanOutWindows:    make(map[string]*ownerWindow),
		},
		config:      cfg,
		auditLogger: cfg.AuditLogger,
	}
	e.now = cfg.Now
	if cfg.Baseline != nil {
		e.SetBaseline(cfg.Baseline)
	}
	return e
}

// Now returns the engine clock for integrations that create incidents on its
// behalf, keeping their timestamps consistent with the lifecycle engine.
func (e *Engine) Now() time.Time {
	return e.now()
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
	// The HPA controller emits FailedGetResourceMetric,
	// FailedComputeMetricsReplicas and FailedGetMetrics for the one condition
	// of having no metrics. Kept apart they were two or three alerts per HPA
	// for a single metrics-server outage.
	switch reason {
	case constant.ReasonFailedComputeMetricsReplicas,
		constant.ReasonFailedGetMetrics:
		return constant.ReasonFailedGetResourceMetric
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
