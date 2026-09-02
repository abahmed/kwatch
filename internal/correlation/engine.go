package correlation

import (
	"strconv"
	"strings"
	"sync"
	"time"

	appsv1lister "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/audit"
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
	// Incident key → concrete Pod name → UID. This protects cleanup when a
	// replacement reuses a name before an old delete tombstone is processed.
	podResourceUIDs map[model.IncidentKey]map[string]string
	serviceLister   corev1lister.ServiceLister
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
		podResourceUIDs:      make(map[model.IncidentKey]map[string]string),
		cleanupCooldown:      make(map[model.IncidentKey]time.Time),
		groupBuffers:         make(map[string]*pendingGroup),
		groupResolveTrackers: make(map[string]*groupResolveTracker),
		groupFlushStates:     make(map[string]*groupFlushState),
		fanOutWindows:        make(map[string]*ownerWindow),
		loggedSkips:          make(map[model.IncidentKey]string),
		massFailures:         make(map[model.IncidentKey]*model.Incident),
	}
	if e.now == nil {
		e.now = clock.Now
	}
	if cfg.Baseline != nil {
		e.SetBaseline(cfg.Baseline)
	}
	return e
}

// SetClock injects the clock used for incident lifecycle timestamps.
func (e *Engine) SetClock(now func() time.Time) {
	if now != nil {
		e.now = now
	}
}

// Now returns the engine clock for integrations that create incidents on its
// behalf, keeping their timestamps consistent with the lifecycle engine.
func (e *Engine) Now() time.Time {
	if e.now != nil {
		return e.now()
	}
	return clock.Now()
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
