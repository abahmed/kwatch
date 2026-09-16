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
	add := func(pipeline *resourcePipeline, name string, missing bool) {
		if pipeline != nil && pipeline.startWorkers && missing {
			unavailable = append(unavailable, name)
		}
	}
	add(c.pod, "pod", c.podLister == nil)
	add(c.pod, "event", c.eventLister == nil)
	add(c.node, "node", c.nodeLister == nil)
	add(c.service, "service", c.serviceLister == nil)
	add(c.endpointSlice, "endpoint-slice", c.endpointSliceLister == nil)
	add(c.ingress, "ingress", c.ingressLister == nil)
	add(c.netpol, "network-policy", c.netpolLister == nil)
	add(c.mwc, "mutating-webhook", c.mwcLister == nil)
	add(c.vwc, "validating-webhook", c.vwcLister == nil)
	add(c.resourceQuota, "resource-quota", c.resourceQuotaLister == nil)
	add(c.limitRange, "limit-range", c.limitRangeLister == nil)
	add(c.namespace, "namespace", c.namespaceLister == nil)
	add(c.lease, "lease", c.leaseLister == nil)
	add(c.deployment, "deployment", c.deployLister == nil)
	add(c.replicaSet, "replicaset", c.rsLister == nil)
	add(c.daemonSet, "daemonset", c.dsLister == nil)
	add(c.statefulSet, "statefulset", c.ssLister == nil)
	add(c.job, "job", c.jobLister == nil)
	add(c.cronJob, "cronjob", c.cronJobLister == nil)
	add(c.hpa, "hpa", c.hpaLister == nil)
	add(c.pdb, "pdb", c.pdbLister == nil)
	add(c.cpPod, "control-plane-pod", c.cpPodLister == nil)
	add(c.pod, "secret", c.secretLister == nil)
	return unavailable
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
