package controller

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/model"
)

// baselineRecorder accumulates the startup Seen set. add converts from the
// typed incident key to the raw ConfigMap wire format; seed handles owner-level
// signals that carry a PodName.
type baselineRecorder struct {
	now        time.Time
	baseline   map[string]map[string]int64
	suppressed map[string]int
	total      int
	max        int
}

func newBaselineRecorder(now time.Time, max int) *baselineRecorder {
	return &baselineRecorder{
		now:        now,
		baseline:   make(map[string]map[string]int64),
		suppressed: map[string]int{},
		max:        max,
	}
}

func (r *baselineRecorder) add(key model.IncidentKey, pod string) {
	ks := string(key)
	if r.total >= r.max {
		return
	}
	if r.baseline[ks] == nil {
		r.baseline[ks] = map[string]int64{}
	}
	if _, exists := r.baseline[ks][pod]; !exists {
		r.total++
	}
	r.baseline[ks][pod] = r.now.Unix()

	if pk := correlation.ParseKey(key); pk.Owner != "" {
		r.suppressed[pk.Owner+"/"+pk.Reason]++
	}
}

func (r *baselineRecorder) seed(sig *event.Signal) {
	ev := event.Event{
		Namespace: sig.Namespace,
		Reason:    sig.Reason,
	}
	key := correlation.IncidentKey(ev, sig.Owner, nil)
	r.add(key, sig.PodName)
}

// seedControlPlane records CP signals under the actual pod name.
func (r *baselineRecorder) seedControlPlane(pod *corev1.Pod, sig *event.Signal) {
	ev := event.Event{
		Namespace: sig.Namespace,
		Reason:    sig.Reason,
	}
	key := correlation.IncidentKey(ev, sig.Owner, nil)
	r.add(key, pod.Name)
}

func (c *Controller) buildSeenSet() {
	pods, err := c.podLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "failed to list pods for Seen set")
		return
	}

	rec := newBaselineRecorder(c.nowTime(), c.maxBaseline)

	for _, pod := range pods {
		c.emitBaseline(rec, pod)
		if sig := handler.DetectPodDeletionIssue(pod, c.nowTime()); sig != nil {
			rec.seed(sig)
		}
	}

	c.seedNodeBaseline(rec)
	c.seedControllers(rec)
	c.seedServices(rec)
	c.seedControllersWithSvc(rec)
	c.seedControlPlaneBaseline(rec)

	if len(rec.baseline) > 0 {
		klog.V(4).InfoS("Seen set built", "count", len(rec.baseline))
		c.handler.SetBaseline(rec.baseline)
	}
	c.handler.ReportStartupSummary(rec.suppressed)
}

// emitBaseline records container, scheduling and node issues for one pod.
func (c *Controller) emitBaseline(rec *baselineRecorder, pod *corev1.Pod) {
	if pod.Status.Phase == corev1.PodSucceeded {
		return
	}
	owner := correlation.ResolveOwnerName(pod, c.rsLister, c.dsLister, c.ssLister)
	if owner == "" {
		return
	}

	statuses := make([]corev1.ContainerStatus, 0,
		len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses))
	statuses = append(statuses, pod.Status.ContainerStatuses...)
	statuses = append(statuses, pod.Status.InitContainerStatuses...)

	hadContainerIssue := false
	for _, cs := range statuses {
		reason := containerIssueReason(&cs)
		if reason == "" {
			continue
		}
		ev := event.Event{Namespace: pod.Namespace, Reason: reason, ContainerName: cs.Name}
		key := correlation.IncidentKey(ev, owner, &model.ContainerState{RestartCount: cs.RestartCount})
		rec.add(key, pod.Name)
		hadContainerIssue = true
	}

	if !hadContainerIssue {
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
				ev := event.Event{Namespace: pod.Namespace, Reason: cond.Reason, ContainerName: "."}
				key := correlation.IncidentKey(ev, owner, nil)
				rec.add(key, pod.Name)
				break
			}
		}
	}
}

// containerIssueReason normalizes a container status into a signal reason,
// skipping transient and benign states.
func containerIssueReason(cs *corev1.ContainerStatus) string {
	if w := cs.State.Waiting; w != nil {
		if w.Reason == "ContainerCreating" || w.Reason == "PodInitializing" {
			return ""
		}
		// Match ContainerReasonsFilter: normalize CrashLoopBackOff to
		// LastTerminationState reason so the baseline key matches the live
		// signal key.
		if w.Reason == "CrashLoopBackOff" && cs.LastTerminationState.Terminated != nil {
			return cs.LastTerminationState.Terminated.Reason
		}
		return w.Reason
	}
	if t := cs.State.Terminated; t != nil {
		if t.ExitCode == 0 || t.Reason == "Completed" {
			return ""
		}
		return t.Reason
	}
	if cs.State.Running != nil && cs.RestartCount > 0 && cs.LastTerminationState.Terminated != nil {
		return cs.LastTerminationState.Terminated.Reason
	}
	return ""
}

// seedNodeBaseline seeds alerting node conditions and pre-populates active node
// incidents so pod suppression is active before any worker starts.
func (c *Controller) seedNodeBaseline(rec *baselineRecorder) {
	var activeNodeIncidents []string
	if len(c.node.synced) > 0 && c.nodeLister != nil {
		nodes, err := c.nodeLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list nodes for baseline seeding")
		} else {
			for _, n := range nodes {
				hasIssue := false
				for _, cond := range n.Status.Conditions {
					if reason := handler.NodeConditionReason(cond); reason != "" {
						ev := event.Event{Reason: reason}
						key := correlation.IncidentKey(ev, n.Name, nil)
						rec.add(key, n.Name)
						hasIssue = true
					}
				}
				if sig := handler.DetectNodeDeletionIssue(n, c.nowTime()); sig != nil {
					rec.seed(sig)
				}
				if hasIssue {
					activeNodeIncidents = append(activeNodeIncidents, n.Name)
				}
			}
		}
	}

	if len(activeNodeIncidents) > 0 {
		c.handler.SetActiveNodeIncidents(activeNodeIncidents)
	}
}
