package handler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type Handler interface {
	ProcessPod(ctx context.Context, key string, deleted bool) error
	ProcessNode(key string, deleted bool) error
	ProcessDeployment(key string, deleted bool) error
	ProcessJob(key string, deleted bool) error
	ProcessDaemonSet(key string, deleted bool) error
	ProcessCronJob(key string, deleted bool) error
	ProcessPodObject(ctx context.Context, pod *corev1.Pod, deleted bool) error
	ProcessNodeObject(node *corev1.Node, deleted bool) error
	ProcessDeploymentObject(deploy *appsv1.Deployment, deleted bool) error
	ProcessJobObject(job *batchv1.Job, deleted bool) error
	ProcessDaemonSetObject(ds *appsv1.DaemonSet, deleted bool) error
	ProcessCronJobObject(cj *batchv1.CronJob, deleted bool) error
	ProcessHorizontalPodAutoscaler(key string, deleted bool) error
	ProcessHorizontalPodAutoscalerObject(hpa *autoscalingv2.HorizontalPodAutoscaler, deleted bool) error
	SetPodLister(lister corev1lister.PodLister)
	SetNodeLister(lister corev1lister.NodeLister)
	SetDeploymentLister(lister appsv1lister.DeploymentLister)
	SetJobLister(lister batchv1lister.JobLister)
	SetReplicaLister(lister appsv1lister.ReplicaSetLister)
	SetDaemonSetLister(lister appsv1lister.DaemonSetLister)
	SetStatefulSetLister(lister appsv1lister.StatefulSetLister)
	SetEventLister(lister corev1lister.EventLister)
	SetCronJobLister(lister batchv1lister.CronJobLister)
	SetHorizontalPodAutoscalerLister(lister autoscalingv2lister.HorizontalPodAutoscalerLister)
	SetSecretLister(lister corev1lister.SecretLister)
	SweepTLSSecrets()
	SetSeen(baseline map[string]map[string]int64)
	SetActiveNodeIncidents(nodeNames []string)
	ClearSeenForPod(namespace, podName string)
	ReportStartupSummary(suppressed map[string]int)
	SetPvcSampler(f func(nodeName string))
}

type handler struct {
	kclient            kubernetes.Interface
	config             *config.Config
	podDetectors       []filter.Detector
	podEnrichers       []filter.Enricher
	containerDetectors []filter.Detector
	containerEnrichers []filter.Enricher
	correlator         *correlation.Engine
	alertManager       *alert.AlertManager
	podLister          corev1lister.PodLister
	nodeLister         corev1lister.NodeLister
	deployLister       appsv1lister.DeploymentLister
	jobLister          batchv1lister.JobLister
	cronJobLister      batchv1lister.CronJobLister
	rsLister           appsv1lister.ReplicaSetLister
	dsLister           appsv1lister.DaemonSetLister
	ssLister           appsv1lister.StatefulSetLister
	eventLister        corev1lister.EventLister
	hpaLister          autoscalingv2lister.HorizontalPodAutoscalerLister
	firstMaxedHPAs     map[string]time.Time
	hpaMu              sync.Mutex
	firstUnavailableDS map[string]time.Time
	dsMu               sync.Mutex
	secretLister       corev1lister.SecretLister
	pvcSampler         func(nodeName string) // optional; set when pvcMonitor is enabled
	now                func() time.Time
}

func NewHandler(
	cli kubernetes.Interface,
	cfg *config.Config,
	correlator *correlation.Engine,
	alertManager *alert.AlertManager) Handler {
	podDetectors := []filter.Detector{
		filter.NamespaceFilter{},
		filter.PodNameFilter{},
		filter.PodStatusFilter{},
	}

	if cfg.PendingPodMonitor.Enabled {
		pendingThreshold := time.Duration(cfg.PendingPodMonitor.Threshold) * time.Second
		if pendingThreshold <= 0 {
			pendingThreshold = 300 * time.Second
		}
		podDetectors = append(podDetectors, filter.PendingPodFilter{Threshold: pendingThreshold})
	}

	podEnrichers := []filter.Enricher{
		filter.PodEventsFilter{},
		filter.PodOwnersFilter{},
	}

	containerDetectors := []filter.Detector{
		filter.NamespaceFilter{},
		filter.PodNameFilter{},
		filter.ContainerNameFilter{},
		filter.ContainerRestartsFilter{},
		filter.ContainerStateFilter{},
		filter.ContainerReasonsFilter{},
		filter.NoiseFilter{},
	}

	containerDetectors = append(containerDetectors,
		filter.ContainerMessageFilter{})

	if cfg.IgnoreDisruptionTerminations == nil || *cfg.IgnoreDisruptionTerminations {
		podDetectors = append([]filter.Detector{filter.DisruptionFilter{}}, podDetectors...)
		containerDetectors = append([]filter.Detector{filter.DisruptionFilter{}}, containerDetectors...)
	}

	containerEnrichers := []filter.Enricher{
		filter.ContainerKillingFilter{},
		filter.PodOwnersFilter{},
		filter.ContainerLogsFilter{},
	}

	return &handler{
		kclient:            cli,
		config:             cfg,
		podDetectors:       podDetectors,
		podEnrichers:       podEnrichers,
		containerDetectors: containerDetectors,
		containerEnrichers: containerEnrichers,
		correlator:         correlator,
		alertManager:       alertManager,
		firstMaxedHPAs:     make(map[string]time.Time),
		firstUnavailableDS: make(map[string]time.Time),
		now:                time.Now,
	}
}

func (h *handler) SetPodLister(lister corev1lister.PodLister) {
	h.podLister = lister
}

func (h *handler) SetNodeLister(lister corev1lister.NodeLister) {
	h.nodeLister = lister
}

func (h *handler) SetDeploymentLister(lister appsv1lister.DeploymentLister) {
	h.deployLister = lister
}

func (h *handler) SetJobLister(lister batchv1lister.JobLister) {
	h.jobLister = lister
}

func (h *handler) SetReplicaLister(lister appsv1lister.ReplicaSetLister) {
	h.rsLister = lister
}

func (h *handler) SetDaemonSetLister(lister appsv1lister.DaemonSetLister) {
	h.dsLister = lister
}

func (h *handler) SetStatefulSetLister(lister appsv1lister.StatefulSetLister) {
	h.ssLister = lister
}

func (h *handler) SetEventLister(lister corev1lister.EventLister) {
	h.eventLister = lister
}

func (h *handler) SetHorizontalPodAutoscalerLister(lister autoscalingv2lister.HorizontalPodAutoscalerLister) {
	h.hpaLister = lister
}

func (h *handler) SetSecretLister(lister corev1lister.SecretLister) {
	h.secretLister = lister
}

func (h *handler) SetPvcSampler(f func(nodeName string)) {
	h.pvcSampler = f
}

func (h *handler) SetCronJobLister(lister batchv1lister.CronJobLister) {
	h.cronJobLister = lister
}

func (h *handler) SetSeen(baseline map[string]map[string]int64) {
	h.correlator.SetSeen(baseline)
}

func (h *handler) SetActiveNodeIncidents(nodeNames []string) {
	h.correlator.SetActiveNodeIncidents(nodeNames)
}

func (h *handler) ClearSeenForPod(namespace, podName string) {
	h.correlator.ClearSeenForPod(namespace, podName)
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
		ID: "startup-baseline", Key: "startup:baseline", Reason: "PreExistingAtStartup",
		Severity: "normal", Count: total,
		Hint: fmt.Sprintf("kwatch started with %d pre-existing issue(s), suppressed from per-incident alerts: %s",
			total, strings.Join(parts, ", ")),
	}
	h.alertManager.NotifyIncident(inc, model.ActionCreate)
}

func (h *handler) report(ev event.Event, owner string, cs *model.ContainerState) {
	inc, action := h.correlator.Process(ev, owner, cs)
	if action != model.ActionSkip {
		h.alertManager.NotifyIncident(inc, action)
	}
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
		Reason:        s.Reason,
		Events:        s.Events,
		Logs:          s.Logs,
		Labels:        s.Labels,
		OwnerKind:     s.OwnerKind,
		RestartCount:  int(s.RestartCount),
		Hint:          s.Hint,
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
