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
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/model"
)

type Handler interface {
	ProcessPod(ctx context.Context, key string, deleted bool) error
	ProcessNode(key string, deleted bool) error
	ProcessDeployment(key string, deleted bool) error
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
	ProcessControlPlanePod(pod *corev1.Pod) error
	SweepControlPlane()
	SweepTLSSecrets()
	// SetListers installs every informer-backed lookup in one call, once the
	// controller has wired its informers.
	SetListers(Listers)
	SetBaseline(baseline map[string]map[string]int64)
	SetActiveNodeIncidents(nodeNames []string)
	ClearBaselineForPod(namespace, podName string)
	ReportStartupSummary(suppressed map[string]int)
	ProcessNodeResourceOvercommit(
		reason, nodeName, hint string,
		severity model.Severity,
	)
	ProcessClusterAutoscalerEvent(ev *corev1.Event)
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

	listers Listers

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

	return &handler{
		kclient:      cli,
		config:       cfg,
		correlator:   correlator,
		alertManager: alertManager,
		fs:           newFirstSeenSet(),
		oomTracker:   oomTr,
		now:          time.Now,

		podDetectors:                  buildPodDetectors(cfg),
		podEnrichers:                  buildPodEnrichers(),
		containerDetectors:            buildContainerDetectors(cfg),
		containerSuppressionEnrichers: buildContainerSuppressionEnrichers(),
		containerDataEnrichers:        buildContainerDataEnrichers(),
	}
}

func (h *handler) ProcessNodeResourceOvercommit(
	reason, nodeName, hint string,
	severity model.Severity,
) {
	if severity == "" {
		severity = model.SeverityWarning
	}
	h.signalEvent(&event.Signal{
		Resource: "node",
		Reason:   reason,
		Hint:     hint,
		NodeName: nodeName,
		Owner:    nodeName,
		Severity: severity,
	})
}

func (h *handler) ReportStartupSummary(suppressed map[string]int) {
	if !h.config.ReportStartupBaseline || len(suppressed) == 0 {
		return
	}
	parts := make([]string, 0, len(suppressed))
	total := 0
	for k, n := range suppressed {
		parts = append(parts, fmt.Sprintf("%s ×%d", k, n))
		total += n
	}
	sort.Strings(parts)
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
		Evidence: model.Evidence{
			Hint: fmt.Sprintf(
				"kwatch started with %d pre-existing issue(s), suppressed "+
					"from per-incident alerts: %s",
				total,
				strings.Join(parts, ", "),
			),
		},
	}

	// This is the one direct send outside the correlation engine, and it is
	// deliberate: the startup summary is a one-off report, not an incident
	// with a lifecycle — nothing will update or resolve it, so there is no
	// state for the engine to own.
	h.alertManager.NotifyIncident(inc, model.ActionCreate, nil)
}

// report feeds one event to the correlation engine. The engine decides and
// announces on its own; notifying here as well is how the live path once
// diverged from every timer-driven path (no audit, no diagnosis).
func (h *handler) report(
	ev event.Event,
	owner string,
	cs *model.ContainerState,
) {
	h.correlator.Process(ev, owner, cs)
}

// signalEvent converts a Signal to an Event and sends it through the
// correlation engine. It applies eventWithConfig and builds a
// ContainerState from the signal fields or uses the pre-built one.
func (h *handler) signalEvent(s *event.Signal) {
	ev := event.Event{
		Resource:      s.Resource,
		PodName:       s.PodName,
		Namespace:     s.Namespace,
		NodeName:      s.NodeName,
		ContainerName: s.Container,
		Image:         s.Image,
		Message:       s.Message,
		Reason:        s.Reason,
		Events:        s.Events,
		Logs:          s.Logs,
		Labels:        s.Labels,
		OwnerKind:     s.OwnerKind,
		RestartCount:  int(s.RestartCount),
		Hint:          s.Hint,
		Facts:         s.Facts,
		Severity:      s.Severity,
	}

	if s.Message != "" && ev.Hint == "" {
		ev.Hint = s.Message
	}

	ev = h.eventWithConfig(ev)

	var cs *model.ContainerState
	if s.ContainerState != nil {
		cs = s.ContainerState
	} else if s.RestartCount > 0 {
		cs = &model.ContainerState{
			RestartCount: s.RestartCount,
		}
	}

	h.report(ev, s.Owner, cs)
}

func (h *handler) eventWithConfig(ev event.Event) event.Event {
	ev.IncludeEvents = h.config.IncludeEvents == nil || *h.config.IncludeEvents
	ev.IncludeLogs = h.config.IncludeLogs == nil || *h.config.IncludeLogs
	return ev
}
