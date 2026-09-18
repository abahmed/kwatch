package controller

import (
	"encoding/json"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

type InformerStatus struct {
	State                  string        `json:"state"`
	LastEvent              time.Time     `json:"lastEvent"`
	LastWatchError         time.Time     `json:"lastWatchError"`
	WatchErrors            int64         `json:"watchErrors"`
	Events                 int64         `json:"events"`
	EventAge               time.Duration `json:"eventAge"`
	WatchHealthy           bool          `json:"watchHealthy"`
	LastError              string        `json:"lastError,omitempty"`
	InformerCount          int           `json:"informerCount"`
	Unsynced               int           `json:"unsynced"`
	WithoutResourceVersion int           `json:"withoutResourceVersion"`
	UnavailableSources     []string      `json:"unavailableSources,omitempty"`
}

func (c *Controller) nowTime() time.Time {
	return c.now()
}

func (c *Controller) InformerStatus() InformerStatus {
	c.informerMu.RLock()
	defer c.informerMu.RUnlock()
	now := c.nowTime()
	status := InformerStatus{
		State:          "unavailable",
		LastEvent:      c.informerLastEvent,
		LastWatchError: c.informerLastWatchError,
		WatchErrors:    c.informerWatchErrors,
		Events:         c.informerEvents,
		LastError:      c.informerLastWatchMessage,
		WatchHealthy: c.informerWatchErrors == 0 ||
			now.Sub(c.informerLastWatchError) > 5*time.Minute,
		InformerCount: len(c.informers),
	}
	for _, informer := range c.informers {
		if !informer.HasSynced() {
			status.Unsynced++
		}
		if informer.LastSyncResourceVersion() == "" {
			status.WithoutResourceVersion++
		}
	}
	status.UnavailableSources = c.unavailableSources()
	if !status.LastEvent.IsZero() {
		status.EventAge = now.Sub(status.LastEvent)
	}
	switch {
	case status.Unsynced > 0:
		status.State = "partial"
	case status.InformerCount > 0 && !status.WatchHealthy:
		status.State = "unavailable"
	case len(status.UnavailableSources) > 0:
		status.State = "degraded"
	case status.InformerCount > 0:
		status.State = "healthy"
	}
	return status
}

// unavailableSources reports active family capabilities that have not been
// assembled yet. Disabled pipelines are intentionally excluded: an optional
// monitor that is turned off is not degraded.
func (c *Controller) unavailableSources() []string {
	var unavailable []string
	for _, requirement := range c.sourceRequirements() {
		if requirement.pipeline != nil &&
			requirement.pipeline.startWorkers &&
			requirement.available != nil && !requirement.available() {
			unavailable = append(unavailable, requirement.name)
		}
	}
	return unavailable
}

func (c *Controller) sourceRequirements() []sourceRequirement {
	return []sourceRequirement{
		{name: "pod", pipeline: c.pod,
			available: func() bool { return c.podLister != nil }},
		{name: "event", pipeline: c.pod,
			available: func() bool { return c.eventLister != nil }},
		{name: "secret", pipeline: c.pod,
			available: func() bool { return c.secretLister != nil }},
		{name: "node", pipeline: c.node,
			available: func() bool { return c.nodeLister != nil }},
		{name: "service", pipeline: c.service,
			available: func() bool { return c.serviceLister != nil }},
		{name: "endpoint-slice", pipeline: c.endpointSlice,
			available: func() bool { return c.endpointSliceLister != nil }},
		{name: "ingress", pipeline: c.ingress,
			available: func() bool { return c.ingressLister != nil }},
		{name: "network-policy", pipeline: c.netpol,
			available: func() bool { return c.netpolLister != nil }},
		{name: "mutating-webhook", pipeline: c.mwc,
			available: func() bool { return c.mwcLister != nil }},
		{name: "validating-webhook", pipeline: c.vwc,
			available: func() bool { return c.vwcLister != nil }},
		{name: "resource-quota", pipeline: c.resourceQuota,
			available: func() bool { return c.resourceQuotaLister != nil }},
		{name: "limit-range", pipeline: c.limitRange,
			available: func() bool { return c.limitRangeLister != nil }},
		{name: "namespace", pipeline: c.namespace,
			available: func() bool { return c.namespaceLister != nil }},
		{name: "lease", pipeline: c.lease,
			available: func() bool { return c.leaseLister != nil }},
		{name: "deployment", pipeline: c.deployment,
			available: func() bool { return c.deployLister != nil }},
		{name: "replicaset", pipeline: c.replicaSet,
			available: func() bool { return c.rsLister != nil }},
		{name: "daemonset", pipeline: c.daemonSet,
			available: func() bool { return c.dsLister != nil }},
		{name: "statefulset", pipeline: c.statefulSet,
			available: func() bool { return c.ssLister != nil }},
		{name: "job", pipeline: c.job,
			available: func() bool { return c.jobLister != nil }},
		{name: "cronjob", pipeline: c.cronJob,
			available: func() bool { return c.cronJobLister != nil }},
		{name: "hpa", pipeline: c.hpa,
			available: func() bool { return c.hpaLister != nil }},
		{name: "pdb", pipeline: c.pdb,
			available: func() bool { return c.pdbLister != nil }},
		{name: "control-plane-pod", pipeline: c.cpPod,
			available: func() bool { return c.cpPodLister != nil }},
	}
}

// StatusJSON implements the health status boundary.
func (c *Controller) StatusJSON() ([]byte, error) {
	return json.Marshal(c.InformerStatus())
}

func (c *Controller) recordInformerEvent() {
	c.informerMu.Lock()
	c.informerEvents++
	metrics.DefaultRegistry().InformerEvents.Add(1)
	c.informerLastEvent = c.nowTime()
	c.informerMu.Unlock()
}

func (c *Controller) recordInformerWatchError(err error) {
	c.informerMu.Lock()
	c.informerWatchErrors++
	metrics.DefaultRegistry().InformerWatchErrors.Add(1)
	c.informerLastWatchError = c.nowTime()
	c.informerLastWatchMessage = safeWatchError(err)
	c.informerMu.Unlock()
	klog.ErrorS(err, "informer watch failed")
}

func safeWatchError(err error) string {
	if err == nil {
		return "watch_failed"
	}
	if apierrors.IsForbidden(err) {
		return "permission_denied"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "timeout") {
		return "watch_timeout"
	}
	return "watch_failed"
}

// allPipelines returns every pipeline in a fixed order for iteration over
// shutdown, cache sync, and worker start.
func (c *Controller) allPipelines() []*resourcePipeline {
	return []*resourcePipeline{
		c.pod, c.node, c.deployment, c.job, c.daemonSet, c.statefulSet,
		c.pdb, c.cronJob, c.hpa, c.service, c.endpointSlice, c.mwc,
		c.vwc, c.ingress, c.netpol, c.cpPod, c.resourceQuota,
		c.limitRange, c.namespace, c.lease, c.replicaSet,
	}
}

// activePipelines returns the pipelines whose watches were wired during New.
func (c *Controller) activePipelines() []*resourcePipeline {
	var active []*resourcePipeline
	for _, pipeline := range c.allPipelines() {
		if pipeline.startWorkers {
			active = append(active, pipeline)
		}
	}
	return active
}
