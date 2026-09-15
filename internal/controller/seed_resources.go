package controller

import (
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	clustermonitor "github.com/abahmed/kwatch/internal/monitor/cluster"
	"github.com/abahmed/kwatch/internal/monitor/network"
	"github.com/abahmed/kwatch/internal/monitor/pod/policy"
	"github.com/abahmed/kwatch/internal/monitor/workload"
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
		if sig := workload.DetectReplicaSetIssue(rs); sig != nil {
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
				if sig := clustermonitor.DetectResourceQuotaIssue(quota); sig != nil {
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
				if sig := clustermonitor.DetectLimitRangeIssue(limitRange); sig != nil {
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
				if sig := clustermonitor.DetectNamespaceIssue(
					namespace, c.nowTime(),
					c.seedThresholds.namespaceSustainedMinutes,
				); sig != nil {
					rec.seed(sig)
				}
				if sig := clustermonitor.DetectPodSecurityLabelIssue(
					namespace,
				); sig != nil {
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
				if sig := clustermonitor.DetectNodeLeaseIssue(
					lease, c.nowTime(),
					c.seedThresholds.nodeLeaseStaleSeconds,
				); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
}

// seedThresholds are the configured sustain windows the seeding pass uses.
type seedThresholds struct {
	namespaceSustainedMinutes int
	nodeLeaseStaleSeconds     int
	pendingPod                time.Duration
	notReady                  time.Duration
	pendingPodEnabled         bool
	notReadyEnabled           bool
}

func newSeedThresholds(runtime config.RuntimeConfig) seedThresholds {
	pending := time.Duration(
		runtime.PendingPodMonitor().Threshold,
	) * time.Second
	if pending <= 0 {
		pending = defaultPendingPodThreshold
	}
	cluster := runtime.ClusterResourceMonitor()
	return seedThresholds{
		namespaceSustainedMinutes: cluster.SustainedMinutes,
		nodeLeaseStaleSeconds:     cluster.NodeLeaseStaleSeconds,
		pendingPod:                pending,
		notReady:                  policy.DefaultNotReadyThreshold,
		pendingPodEnabled:         runtime.PendingPodMonitor().Enabled,
		notReadyEnabled:           runtime.NotReadyMonitor().Enabled,
	}
}

// defaultPendingPodThreshold mirrors the pod pipeline's fallback, so a seeded
// key matches the live one when the config leaves the threshold unset.
const defaultPendingPodThreshold = 300 * time.Second

func (c *Controller) seedDaemonSets(rec *baselineRecorder) {
	if c.dsLister != nil {
		dss, err := c.dsLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list daemonsets for baseline seeding")
		} else {
			for _, ds := range dss {
				if sig := workload.DetectDaemonSetIssue(ds); sig != nil {
					rec.seed(sig)
				}
				for _, sig := range workload.DetectDaemonSetConditions(ds) {
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
				if sig := workload.DetectStatefulSetIssue(ss); sig != nil {
					rec.seed(sig)
				}
				for _, sig := range workload.DetectStatefulSetConditions(ss) {
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
				if sig := workload.DetectPDBIssue(pdb); sig != nil {
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
				sig := workload.DetectDeploymentIssue(deploy)
				if sig == nil {
					sig = workload.DetectDeploymentUnavailable(deploy)
				}
				if sig != nil {
					rec.seed(sig)
				}
				for _, conditionSig := range workload.DetectDeploymentConditions(deploy) {
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
				if sig := workload.DetectJobIssue(job); sig != nil {
					rec.seed(sig)
				}
				if sig := workload.DetectJobExecutionIssue(job, c.nowTime()); sig != nil {
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
				if sig := workload.DetectCronJobIssue(
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
				for _, sig := range workload.DetectHPAIssues(hpa) {
					rec.seed(sig)
				}
			}
		}
	}
}

func (c *Controller) seedServices(rec *baselineRecorder) {
	// Services — seed service-endpoint issues
	if c.service != nil && c.service.startWorkers &&
		c.serviceLister != nil &&
		c.endpointSliceLister != nil {
		svcs, err := c.serviceLister.List(labels.Everything())
		if err != nil {
			klog.ErrorS(err, "failed to list services for baseline seeding")
		} else {
			for _, svc := range svcs {
				if sig := network.DetectServiceStatusIssue(
					svc,
					c.nowTime(),
					network.DefaultServiceSustainedSeconds,
				); sig != nil {
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
				if sig := network.DetectServiceEndpointIssue(
					svc,
					epSlices,
				); sig != nil {
					rec.seed(sig)
				}
				if sig := network.DetectServicePortIssue(svc, epSlices); sig != nil {
					rec.seed(sig)
				}
			}
		}
	}
}
