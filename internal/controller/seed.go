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

func (c *Controller) buildSeenSet() {
	pods, err := c.podLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "failed to list pods for Seen set")
		return
	}
	now := time.Now()
	baseline := make(map[string]map[string]int64)

	suppressed := map[string]int{}
	total := 0
	// baseline is keyed by raw strings (the ConfigMap wire format); add
	// converts from the typed incident key at the boundary.
	add := func(key model.IncidentKey, pod string) {
		ks := string(key)
		if total >= c.maxBaseline {
			return
		}
		if baseline[ks] == nil {
			baseline[ks] = map[string]int64{}
		}
		if _, exists := baseline[ks][pod]; !exists {
			total++
		}
		baseline[ks][pod] = now.Unix()

		// Derive owner/reason key for the suppressed count from the incident key.
		if pk := correlation.ParseKey(key); pk.Owner != "" {
			suppressed[pk.Owner+"/"+pk.Reason]++
		}
	}

	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodSucceeded {
			continue
		}
		owner := correlation.ResolveOwnerName(pod, c.rsLister, c.dsLister, c.ssLister)
		if owner == "" {
			continue
		}

		statuses := make([]corev1.ContainerStatus, 0,
			len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses))
		statuses = append(statuses, pod.Status.ContainerStatuses...)
		statuses = append(statuses, pod.Status.InitContainerStatuses...)

		hadContainerIssue := false
		for _, cs := range statuses {
			var reason string
			if w := cs.State.Waiting; w != nil {
				if w.Reason == "ContainerCreating" || w.Reason == "PodInitializing" {
					continue
				}
				// Match ContainerReasonsFilter: normalize CrashLoopBackOff
				// to LastTerminationState reason so the baseline key matches
				// the live signal key.
				if w.Reason == "CrashLoopBackOff" && cs.LastTerminationState.Terminated != nil {
					reason = cs.LastTerminationState.Terminated.Reason
				} else {
					reason = w.Reason
				}
			} else if t := cs.State.Terminated; t != nil {
				if t.ExitCode == 0 || t.Reason == "Completed" {
					continue
				}
				reason = t.Reason
			} else if cs.State.Running != nil && cs.RestartCount > 0 && cs.LastTerminationState.Terminated != nil {
				reason = cs.LastTerminationState.Terminated.Reason
			}
			if reason == "" {
				continue
			}
			ev := event.Event{Namespace: pod.Namespace, Reason: reason, ContainerName: cs.Name}
			key := correlation.IncidentKey(ev, owner, &model.ContainerState{RestartCount: cs.RestartCount})
			add(key, pod.Name)
			hadContainerIssue = true
		}

		if !hadContainerIssue {
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
					ev := event.Event{Namespace: pod.Namespace, Reason: cond.Reason, ContainerName: "."}
					key := correlation.IncidentKey(ev, owner, nil)
					add(key, pod.Name)
					break
				}
			}
		}
	}

	// Seed alerting node conditions into the baseline
	// and collect broken node names for pre-populating activeNodeIncidents
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
						add(key, n.Name)
						hasIssue = true
					}
				}
				if hasIssue {
					activeNodeIncidents = append(activeNodeIncidents, n.Name)
				}
			}
		}
	}

	// Pre-populate activeNodeIncidents so pod suppression is active
	// before any worker starts (timing race prevention).
	if len(activeNodeIncidents) > 0 {
		c.handler.SetActiveNodeIncidents(activeNodeIncidents)
	}

	// Seed controller resource issues into the baseline.
	// Live owner-level signals may carry a PodName (services and webhooks set
	// it to the resource name), so seed under the same key that the live
	// isBaselined(key, ev.PodName) lookup will use.
	seedSignal := func(sig *event.Signal, name string) {
		ev := event.Event{
			Namespace: sig.Namespace,
			Reason:    sig.Reason,
		}
		key := correlation.IncidentKey(ev, sig.Owner, nil)
		add(key, sig.PodName)
	}

	// DaemonSets
	if c.dsLister != nil {
		dss, err := c.dsLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list daemonsets for baseline seeding")
		} else {
			for _, ds := range dss {
				if sig := handler.DetectDaemonSetIssue(ds); sig != nil {
					seedSignal(sig, ds.Name)
				}
			}
		}
	}

	// StatefulSets
	if c.ssLister != nil {
		sss, err := c.ssLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list statefulsets for baseline seeding")
		} else {
			for _, ss := range sss {
				if sig := handler.DetectStatefulSetIssue(ss); sig != nil {
					seedSignal(sig, ss.Name)
				}
			}
		}
	}

	// PodDisruptionBudgets
	if c.pdbLister != nil {
		pdbs, err := c.pdbLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list pod disruption budgets for baseline seeding")
		} else {
			for _, pdb := range pdbs {
				if sig := handler.DetectPdbIssue(pdb); sig != nil {
					seedSignal(sig, pdb.Name)
				}
			}
		}
	}

	// Deployments
	if c.deployLister != nil {
		deploys, err := c.deployLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list deployments for baseline seeding")
		} else {
			for _, deploy := range deploys {
				if sig := handler.DetectDeploymentIssue(deploy); sig != nil {
					seedSignal(sig, deploy.Name)
				} else if sig := handler.DetectDeploymentUnavailable(deploy); sig != nil {
					seedSignal(sig, deploy.Name)
				}
			}
		}
	}

	// Jobs
	if c.jobLister != nil {
		jobs, err := c.jobLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list jobs for baseline seeding")
		} else {
			for _, job := range jobs {
				if sig := handler.DetectJobIssue(job); sig != nil {
					seedSignal(sig, job.Name)
				}
			}
		}
	}

	// CronJobs
	if c.cronJobLister != nil {
		cjs, err := c.cronJobLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list cronjobs for baseline seeding")
		} else {
			for _, cj := range cjs {
				if sig := handler.DetectCronJobIssue(cj); sig != nil {
					seedSignal(sig, cj.Name)
				}
			}
		}
	}

	// HPAs — seed both scaling errors and maxed-out conditions
	if c.hpaLister != nil {
		hpas, err := c.hpaLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list horizontal pod autoscalers for baseline seeding")
		} else {
			for _, hpa := range hpas {
				for _, sig := range handler.DetectHPAIssues(hpa) {
					seedSignal(sig, hpa.Name)
				}
			}
		}
	}

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
					seedSignal(sig, svc.Name)
				}
			}
		}
	}

	// Admission webhooks — seed webhook-backend issues
	if c.mwcLister != nil {
		hasSvc := func(ns, name string) bool {
			if c.serviceLister == nil {
				return true
			}
			_, err := c.serviceLister.Services(ns).Get(name)
			return err == nil
		}
		mwcs, err := c.mwcLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list mutating webhook configurations for baseline seeding")
		} else {
			for _, mwc := range mwcs {
				for _, sig := range handler.DetectMutatingWebhookIssue(mwc, hasSvc) {
					seedSignal(sig, mwc.Name)
				}
			}
		}
	}
	if c.vwcLister != nil {
		hasSvc := func(ns, name string) bool {
			if c.serviceLister == nil {
				return true
			}
			_, err := c.serviceLister.Services(ns).Get(name)
			return err == nil
		}
		vwcs, err := c.vwcLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list validating webhook configurations for baseline seeding")
		} else {
			for _, vwc := range vwcs {
				for _, sig := range handler.DetectValidatingWebhookIssue(vwc, hasSvc) {
					seedSignal(sig, vwc.Name)
				}
			}
		}
	}

	// Ingresses — seed ingress-backend issues
	if c.ingressLister != nil {
		hasSvc := func(ns, name string) bool {
			if c.serviceLister == nil {
				return true
			}
			_, err := c.serviceLister.Services(ns).Get(name)
			return err == nil
		}
		ings, err := c.ingressLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list ingresses for baseline seeding")
		} else {
			for _, ing := range ings {
				for _, sig := range handler.DetectIngressIssue(ing, hasSvc) {
					seedSignal(sig, ing.Name)
				}
			}
		}
	}

	// NetworkPolicies — seed restrictive-policy issues
	if c.netpolLister != nil {
		policies, err := c.netpolLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list network policies for baseline seeding")
		} else {
			for _, policy := range policies {
				if sig := handler.DetectNetworkPolicyIssue(policy); sig != nil {
					seedSignal(sig, policy.Name)
				}
			}
		}
	}

	// Control-plane — seed control-plane component failures.
	// Unlike other owner-level signals, CP signals carry PodName, so we
	// seed with the actual pod name (not "") to match the live lookup.
	if c.cpWatchEnabled && c.cpPodLister != nil {
		pods, err := c.cpPodLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list control-plane pods for baseline seeding")
		} else {
			for _, pod := range pods {
				if sig := handler.DetectControlPlanePodIssue(pod); sig != nil {
					ev := event.Event{
						Namespace: sig.Namespace,
						Reason:    sig.Reason,
					}
					key := correlation.IncidentKey(ev, sig.Owner, nil)
					add(key, pod.Name)
				}
			}
		}
	}

	if len(baseline) > 0 {
		klog.V(4).InfoS("Seen set built", "count", len(baseline))
		c.handler.SetBaseline(baseline)
	}
	c.handler.ReportStartupSummary(suppressed)
}
