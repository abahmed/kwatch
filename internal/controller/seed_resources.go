package controller

import (
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/handler"
)

func (c *Controller) seedControllers(rec *baselineRecorder) {
	c.seedDaemonSets(rec)
	c.seedStatefulSets(rec)
	c.seedPdbs(rec)
	c.seedDeployments(rec)
	c.seedJobs(rec)
	c.seedCronJobs(rec)
	c.seedHPAs(rec)
	c.seedNetworkPolicies(rec)
	c.seedClusterResources(rec)
	c.seedReplicaSets(rec)
}

func (c *Controller) seedReplicaSets(rec *baselineRecorder) {
	if c.rsLister == nil {
		return
	}
	sets, err := c.rsLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "failed to list replicasets for baseline seeding")
		return
	}
	for _, rs := range sets {
		if sig := handler.DetectReplicaSetIssue(rs); sig != nil {
			rec.seed(sig)
		}
	}
}

func (c *Controller) seedClusterResources(rec *baselineRecorder) {
	if c.resourceQuotaLister != nil {
		quotas, err := c.resourceQuotaLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list resource quotas for baseline seeding")
		} else {
			for _, quota := range quotas {
				if sig := handler.DetectResourceQuotaIssue(quota); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
	if c.limitRangeLister != nil {
		limitRanges, err := c.limitRangeLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list limit ranges for baseline seeding")
		} else {
			for _, limitRange := range limitRanges {
				if sig := handler.DetectLimitRangeIssue(limitRange); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
	if c.namespaceLister != nil {
		namespaces, err := c.namespaceLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list namespaces for baseline seeding")
		} else {
			for _, namespace := range namespaces {
				if sig := handler.DetectNamespaceIssue(namespace, c.nowTime(), 0); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
	if c.leaseLister != nil {
		leases, err := c.leaseLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list node leases for baseline seeding")
		} else {
			for _, lease := range leases {
				if sig := handler.DetectNodeLeaseIssue(lease, c.nowTime(), 0); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
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
				for _, sig := range handler.DetectDaemonSetConditions(ds) {
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
				for _, sig := range handler.DetectStatefulSetConditions(ss) {
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
			klog.ErrorS(
				err,
				"failed to list pod disruption budgets for baseline seeding",
			)
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
				sig := handler.DetectDeploymentIssue(deploy)
				if sig == nil {
					sig = handler.DetectDeploymentUnavailable(deploy)
				}
				if sig != nil {
					rec.seed(sig)
				}
				for _, conditionSig := range handler.DetectDeploymentConditions(deploy) {
					rec.seed(conditionSig)
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
				if sig := handler.DetectJobExecutionIssue(job, c.nowTime()); sig != nil {
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
				if sig := handler.DetectCronJobIssue(
					cj,
					c.nowTime(),
				); sig != nil {
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
			klog.ErrorS(
				err,
				"failed to list horizontal pod autoscalers for baseline "+
					"seeding",
			)
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
				if sig := handler.DetectServiceStatusIssue(svc, c.nowTime()); sig != nil {
					rec.seed(sig)
				}
				sel := labels.Set{
					"kubernetes.io/service-name": svc.Name,
				}.AsSelector()
				epSlices, err := c.endpointSliceLister.EndpointSlices(
					svc.Namespace,
				).List(
					sel,
				)
				if err != nil {
					klog.ErrorS(
						err,
						"failed to list endpoint slices for baseline seeding",
						"service",
						svc.Name,
						"namespace",
						svc.Namespace,
					)
					continue
				}
				if sig := handler.DetectServiceEndpointIssue(
					svc,
					epSlices,
				); sig != nil {
					rec.seed(sig)
				}
				if sig := handler.DetectServicePortIssue(svc, epSlices); sig != nil {
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

func (c *Controller) seedMwcs(
	rec *baselineRecorder,
	hasSvc func(ns, name string) bool,
) {
	// Admission webhooks — seed webhook-backend issues
	if c.mwcLister != nil {
		mwcs, err := c.mwcLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(
				err,
				"failed to list mutating webhook configurations for baseline "+
					"seeding",
			)
		} else {
			for _, mwc := range mwcs {
				sigs := handler.DetectMutatingWebhookIssue(mwc, hasSvc)
				for _, sig := range sigs {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedVwcs(
	rec *baselineRecorder,
	hasSvc func(ns, name string) bool,
) {
	if c.vwcLister != nil {
		vwcs, err := c.vwcLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(
				err,
				"failed to list validating webhook configurations for "+
					"baseline seeding",
			)
		} else {
			for _, vwc := range vwcs {
				sigs := handler.DetectValidatingWebhookIssue(vwc, hasSvc)
				for _, sig := range sigs {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedIngresses(
	rec *baselineRecorder,
	hasSvc func(ns, name string) bool,
) {
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
			klog.ErrorS(
				err,
				"failed to list network policies for baseline seeding",
			)
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
	if c.cpPod.startWorkers && c.cpPodLister != nil {
		pods, err := c.cpPodLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(
				err,
				"failed to list control-plane pods for baseline seeding",
			)
		} else {
			for _, pod := range pods {
				if sig := handler.DetectControlPlanePodIssue(pod); sig != nil {
					rec.seedControlPlane(pod, sig)
				}
			}
		}
	}
}
