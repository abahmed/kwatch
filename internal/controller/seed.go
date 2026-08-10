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

	rec := newBaselineRecorder(time.Now(), c.maxBaseline)

	for _, pod := range pods {
		c.emitBaseline(rec, pod)
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
	if c.nodesSynced != nil && c.nodeLister != nil {
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

func (c *Controller) seedControllers(rec *baselineRecorder) {
	c.seedDaemonSets(rec)
	c.seedStatefulSets(rec)
	c.seedPdbs(rec)
	c.seedDeployments(rec)
	c.seedJobs(rec)
	c.seedCronJobs(rec)
	c.seedHPAs(rec)
	c.seedNetworkPolicies(rec)
}

func (c *Controller) seedDaemonSets(rec *baselineRecorder) {
	if c.dsLister != nil {
		dss, err := c.dsLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list daemonsets for baseline seeding")
		} else {
			for _, ds := range dss {
				if sig := handler.DetectDaemonSetIssue(ds); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedStatefulSets(rec *baselineRecorder) {
	if c.ssLister != nil {
		sss, err := c.ssLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list statefulsets for baseline seeding")
		} else {
			for _, ss := range sss {
				if sig := handler.DetectStatefulSetIssue(ss); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedPdbs(rec *baselineRecorder) {
	if c.pdbLister != nil {
		pdbs, err := c.pdbLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list pod disruption budgets for baseline seeding")
		} else {
			for _, pdb := range pdbs {
				if sig := handler.DetectPdbIssue(pdb); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedDeployments(rec *baselineRecorder) {
	if c.deployLister != nil {
		deploys, err := c.deployLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list deployments for baseline seeding")
		} else {
			for _, deploy := range deploys {
				if sig := handler.DetectDeploymentIssue(deploy); sig != nil {
					rec.seed(sig)
				} else if sig := handler.DetectDeploymentUnavailable(deploy); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedJobs(rec *baselineRecorder) {
	if c.jobLister != nil {
		jobs, err := c.jobLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list jobs for baseline seeding")
		} else {
			for _, job := range jobs {
				if sig := handler.DetectJobIssue(job); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedCronJobs(rec *baselineRecorder) {
	if c.cronJobLister != nil {
		cjs, err := c.cronJobLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list cronjobs for baseline seeding")
		} else {
			for _, cj := range cjs {
				if sig := handler.DetectCronJobIssue(cj); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedHPAs(rec *baselineRecorder) {
	// HPAs — seed both scaling errors and maxed-out conditions
	if c.hpaLister != nil {
		hpas, err := c.hpaLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list horizontal pod autoscalers for baseline seeding")
		} else {
			for _, hpa := range hpas {
				for _, sig := range handler.DetectHPAIssues(hpa) {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedServices(rec *baselineRecorder) {
	// Services — seed service-endpoint issues
	if c.serviceLister != nil && c.endpointSliceLister != nil {
		svcs, err := c.serviceLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list services for baseline seeding")
		} else {
			for _, svc := range svcs {
				sel := labels.Set{"kubernetes.io/service-name": svc.Name}.AsSelector()
				epSlices, err := c.endpointSliceLister.EndpointSlices(svc.Namespace).List(sel)
				if err != nil {
					klog.ErrorS(err, "failed to list endpoint slices for baseline seeding", "service", svc.Name, "namespace", svc.Namespace)
					continue
				}
				if sig := handler.DetectServiceEndpointIssue(svc, epSlices); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
}

// hasService is a helper seeded by controllers that reference a service.
func (c *Controller) hasService() func(ns, name string) bool {
	return func(ns, name string) bool {
		if c.serviceLister == nil {
			return true
		}
		_, err := c.serviceLister.Services(ns).Get(name)
		return err == nil
	}
}

// seedControllersWithSvc seeds MWC/VWC and NetworkPolicies
func (c *Controller) seedControllersWithSvc(rec *baselineRecorder) {
	hasSvc := c.hasService()
	c.seedMwcs(rec, hasSvc)
	c.seedVwcs(rec, hasSvc)
	c.seedIngresses(rec, hasSvc)
}

func (c *Controller) seedMwcs(rec *baselineRecorder, hasSvc func(ns, name string) bool) {
	// Admission webhooks — seed webhook-backend issues
	if c.mwcLister != nil {
		mwcs, err := c.mwcLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list mutating webhook configurations for baseline seeding")
		} else {
			for _, mwc := range mwcs {
				for _, sig := range handler.DetectMutatingWebhookIssue(mwc, hasSvc) {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedVwcs(rec *baselineRecorder, hasSvc func(ns, name string) bool) {
	if c.vwcLister != nil {
		vwcs, err := c.vwcLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list validating webhook configurations for baseline seeding")
		} else {
			for _, vwc := range vwcs {
				for _, sig := range handler.DetectValidatingWebhookIssue(vwc, hasSvc) {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedIngresses(rec *baselineRecorder, hasSvc func(ns, name string) bool) {
	// Ingresses — seed ingress-backend issues
	if c.ingressLister != nil {
		ings, err := c.ingressLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list ingresses for baseline seeding")
		} else {
			for _, ing := range ings {
				for _, sig := range handler.DetectIngressIssue(ing, hasSvc) {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedNetworkPolicies(rec *baselineRecorder) {
	// NetworkPolicies — seed restrictive-policy issues
	if c.netpolLister != nil {
		policies, err := c.netpolLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list network policies for baseline seeding")
		} else {
			for _, policy := range policies {
				if sig := handler.DetectNetworkPolicyIssue(policy); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedControlPlaneBaseline(rec *baselineRecorder) {
	// Control-plane — seed CP component failures. Unlike other owner-level
	// signals, CP signals carry PodName, so we seed with the actual pod name.
	if c.cpWatchEnabled && c.cpPodLister != nil {
		pods, err := c.cpPodLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list control-plane pods for baseline seeding")
		} else {
			for _, pod := range pods {
				if sig := handler.DetectControlPlanePodIssue(pod); sig != nil {
					rec.seedControlPlane(pod, sig)
				}
			}
		}
	}
}
