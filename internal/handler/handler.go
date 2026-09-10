package handler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

type Handler interface {
	ProcessPod(ctx context.Context, key string, deleted bool) error
	ProcessNode(key string, deleted bool) error
	ProcessDeployment(key string, deleted bool) error
	ProcessReplicaSet(key string, deleted bool) error
	ProcessJob(key string, deleted bool) error
	ProcessDaemonSet(key string, deleted bool) error
	ProcessCronJob(key string, deleted bool) error
	ProcessStatefulSet(key string, deleted bool) error
	ProcessPdb(key string, deleted bool) error
	ProcessHorizontalPodAutoscaler(key string, deleted bool) error
	ProcessMutatingWebhookConfiguration(key string, deleted bool) error
	ProcessValidatingWebhookConfiguration(key string, deleted bool) error
	ProcessService(key string, deleted bool) error
	ProcessNetworkPolicy(key string, deleted bool) error
	ProcessIngress(key string, deleted bool) error
	ProcessResourceQuota(key string, deleted bool) error
	ProcessLimitRange(key string, deleted bool) error
	ProcessNamespace(key string, deleted bool) error
	ProcessLease(key string, deleted bool) error
	ProcessControlPlanePod(pod *corev1.Pod) error
	SweepControlPlane()
	SweepTLSSecrets()
	// SetListers installs every informer-backed lookup in one call, once the
	// controller has wired its informers.
	SetListers(Listers)
	// SetClock installs the clock used by time-sensitive detectors.
	SetClock(func() time.Time)
	// Owners is the pod-ownership resolver this pipeline keys incidents by,
	// for monitors outside this package that emit pod incidents. Sharing it
	// is what keeps a kubelet-derived incident and a status-derived one about
	// the same Deployment from arriving as two alerts.
	Owners() observe.OwnerResolver
	SetNamespaceScope(namespaces []string, all bool)
	SetBaseline(baseline map[string]map[string]int64)
	SetActiveNodeIncidents(nodeNames []string)
	ClearBaselineForPod(namespace, podName string, owner model.ObjectRef)
	ReportStartupSummary(suppressed map[string]int)
	ProcessNodeResourceOvercommit(
		reason, nodeName, hint string,
		severity model.Severity,
	)
	ProcessClusterAutoscalerEvent(ev *corev1.Event)
	ProcessWarningEvent(ev *corev1.Event)
}

// handler is the central event processor: informers call its Process*
// methods, which run detectors/enrichers (filter package) over observed
// objects and forward resulting signals through the correlation engine to
// the alert manager.
type handler struct {
	kclient      kubernetes.Interface
	config       *config.Config
	correlator   *correlation.Engine
	alertManager *alert.AlertManager
	oomTracker   *oomTracker
	now          func() time.Time

	podDetectors                  []filter.Detector
	podEnrichers                  []filter.Enricher
	containerDetectors            []filter.Detector
	containerSuppressionEnrichers []filter.Enricher
	containerDataEnrichers        []filter.Enricher

	listers           Listers
	namespaceScope    map[string]struct{}
	namespaceScopeAll bool

	// reconciler derives recovery from what each object was last found to be
	// wrong with. See reconcile.go.
	reconciler *reconciler

	// logCache keeps a container's log tail across the passes that re-report
	// the same crash loop.
	logCache *filter.LogCache

	fs firstSeenSet
}

func NewHandler(
	cli kubernetes.Interface,
	cfg *config.Config,
	correlator *correlation.Engine,
	alertManager *alert.AlertManager) *handler {
	var oomTr *oomTracker
	if cfg.OomMonitor.Enabled {
		oomTr = newOomTracker(
			cfg.OomMonitor.Threshold,
			time.Duration(cfg.OomMonitor.WindowMinutes)*time.Minute,
		)
	}

	h := &handler{
		kclient:      cli,
		config:       cfg,
		correlator:   correlator,
		alertManager: alertManager,
		fs:           newFirstSeenSet(),
		oomTracker:   oomTr,
		now:          time.Now,

		reconciler:                    newReconciler(),
		podDetectors:                  buildPodDetectors(cfg),
		podEnrichers:                  buildPodEnrichers(),
		containerDetectors:            buildContainerDetectors(cfg),
		containerSuppressionEnrichers: buildContainerSuppressionEnrichers(),
		containerDataEnrichers:        buildContainerDataEnrichers(),
	}
	// The cache reads the handler's clock rather than the wall clock, so a
	// test that moves time forward expires cached log tails with it.
	h.logCache = filter.NewLogCache(func() time.Time { return h.now() })
	return h
}

// SetClock installs the clock shared by handler detectors and trackers.
func (h *handler) SetClock(now func() time.Time) {
	if now == nil {
		return
	}
	h.now = now
	if h.oomTracker != nil {
		h.oomTracker.SetClock(now)
	}
}

func (h *handler) ProcessNodeResourceOvercommit(
	reason, nodeName, hint string,
	severity model.Severity,
) {
	if severity == "" {
		severity = model.SeverityWarning
	}
	h.observe(
		observe.NodeNamed(nodeName, reason).
			WithSeverity(severity).WithHint(hint),
	)
}

func (h *handler) ReportStartupSummary(suppressed map[string]int) {
	if !h.config.ReportStartupBaseline || len(suppressed) == 0 {
		return
	}
	hint, total := startupSummaryHint(suppressed)
	inc := &model.Incident{
		Subject: model.Subject{
			ID:     "startup-baseline",
			Key:    "startup:baseline",
			Reason: constant.ReasonPreExistingAtStartup,
		},
		Status: model.Status{
			Severity: model.SeverityNormal,
			Count:    total,
		},
		Evidence: model.Evidence{Hint: hint},
	}

	// This is the one direct send outside the correlation engine, and it is
	// deliberate: the startup summary is a one-off report, not an incident
	// with a lifecycle — nothing will update or resolve it, so there is no
	// state for the engine to own.
	h.alertManager.NotifyIncident(inc, model.ActionCreate, nil)
}

// maxStartupReasons bounds the reasons named in the startup summary.
const maxStartupReasons = 8

// startupSummaryHint condenses the suppressed baseline into what a reader can
// take in: how many issues, of which kinds, across how many workloads. Keys
// are "<owner path>/<reason>". Listing every one -- three hundred lines cut
// off mid-word by the chat provider -- said "a lot" and nothing else.
func startupSummaryHint(suppressed map[string]int) (string, int) {
	byReason := make(map[string]int)
	owners := make(map[string]bool)
	total := 0
	for key, n := range suppressed {
		total += n
		reason := key
		owner := ""
		if i := strings.LastIndex(key, "/"); i >= 0 {
			reason, owner = key[i+1:], key[:i]
		}
		byReason[reason] += n
		if owner != "" {
			owners[owner] = true
		}
	}
	reasons := make([]string, 0, len(byReason))
	for reason := range byReason {
		reasons = append(reasons, reason)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if byReason[reasons[i]] != byReason[reasons[j]] {
			return byReason[reasons[i]] > byReason[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})
	parts := make([]string, 0, maxStartupReasons+1)
	for i, reason := range reasons {
		if i == maxStartupReasons {
			parts = append(parts,
				fmt.Sprintf("+%d other kinds", len(reasons)-i))
			break
		}
		parts = append(parts, fmt.Sprintf("%s ×%d", reason, byReason[reason]))
	}
	hint := fmt.Sprintf(
		"kwatch started with %d pre-existing issue(s), not re-alerted",
		total,
	)
	if len(owners) > 0 {
		hint += fmt.Sprintf(" across %d workloads", len(owners))
	}
	return hint + ": " + strings.Join(parts, ", "), total
}

// observe feeds one observation to the correlation engine. The engine decides
// and announces on its own; notifying here as well is how the live path once
// diverged from every timer-driven path (no audit, no diagnosis).
//
// The only thing added here is the evidence policy, which is the one part of
// an alert only this package knows: whether this deployment was configured to
// collect logs and events at all.
func (h *handler) observe(obs *model.Observation) {
	if obs == nil {
		return
	}
	obs.IncludeEvents =
		h.config.IncludeEvents == nil || *h.config.IncludeEvents
	obs.IncludeLogs = h.config.IncludeLogs == nil || *h.config.IncludeLogs
	h.correlator.Process(obs)
}

// Owners implements Handler.
func (h *handler) Owners() observe.OwnerResolver {
	return observe.PodOwners{
		RS: h.listers.RS,
		DS: h.listers.DS,
		SS: h.listers.SS,
	}
}
