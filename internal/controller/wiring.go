package controller

import (
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

// wireNode sets up the node informer when either monitor is enabled.
func (c *Controller) wireNode(runtime config.RuntimeConfig, fs factorySet) {
	if runtime.NodeMonitor().Enabled || runtime.NodeResourceMonitor().Enabled {
		c.nodeLister = fs.nodeLister()

		if runtime.NodeMonitor().Enabled {
			c.watch(c.node, fs.nodeInformer())
		}
	}
}

func (c *Controller) wireRollout(runtime config.RuntimeConfig, fs factorySet) {
	if !runtime.RolloutMonitor().Enabled {
		return
	}
	c.deployLister = fs.deployLister()
	c.watch(c.deployment, fs.deployInformers()...)
}

func (c *Controller) wireJobs(runtime config.RuntimeConfig, fs factorySet) {
	if !runtime.JobMonitor().Enabled {
		return
	}
	c.jobLister = fs.jobLister()
	c.watch(c.job, fs.jobInformers()...)
}

func (c *Controller) wireDaemonSetMonitor(
	runtime config.RuntimeConfig, fs factorySet,
) {
	if !runtime.DaemonSetMonitor().Enabled {
		return
	}
	c.watch(c.daemonSet, fs.dsInformers()...)
}

func (c *Controller) wireCronJobs(runtime config.RuntimeConfig, fs factorySet) {
	if !runtime.CronJobMonitor().Enabled {
		return
	}
	c.cronJobLister = fs.cronJobLister()
	c.watch(c.cronJob, fs.cronJobInformers()...)
}

func (c *Controller) wireHPA(runtime config.RuntimeConfig, fs factorySet) {
	if !runtime.HpaMonitor().Enabled {
		return
	}
	c.hpaLister = fs.hpaLister()
	c.watch(c.hpa, fs.hpaInformers()...)
}

func (c *Controller) wireService(runtime config.RuntimeConfig, fs factorySet) {
	if !runtime.ServiceMonitor().Enabled && !runtime.IngressMonitor().Enabled &&
		!runtime.AdmissionWebhookMonitor().Enabled {
		return
	}
	c.serviceLister = fs.serviceLister()

	serviceInformers := fs.serviceInformers()
	for _, inf := range serviceInformers {
		c.service.synced = append(c.service.synced, inf.HasSynced)
	}

	for _, inf := range serviceInformers {
		inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				c.recordChange(kwcontext.ChangeCreate, "service", obj)
				if runtime.ServiceMonitor().Enabled {
					c.service.enqueue(obj)
				}
				c.enqueueServiceDependents(obj)
			},
			UpdateFunc: func(old, obj interface{}) {
				c.recordChangeUpdate("service", old, obj)
				if runtime.ServiceMonitor().Enabled {
					c.service.enqueue(obj)
				}
				c.enqueueServiceDependents(obj)
			},
			DeleteFunc: func(obj interface{}) {
				c.recordChange(kwcontext.ChangeDelete, "service", obj)
				if runtime.ServiceMonitor().Enabled {
					c.service.enqueue(obj)
				}
				c.enqueueServiceDependents(obj)
			},
		})
	}
	if !runtime.ServiceMonitor().Enabled {
		c.wireEndpointSlices(runtime, fs)
		return
	}
	c.service.startWorkers = true
	c.wireEndpointSlices(runtime, fs)
}

func (c *Controller) wireAdmissionWebhooks(
	runtime config.RuntimeConfig, fs factorySet,
) {
	if !runtime.AdmissionWebhookMonitor().Enabled {
		return
	}
	mwcLister := fs.mwcLister()
	vwcLister := fs.vwcLister()

	c.mwcLister = mwcLister
	c.vwcLister = vwcLister

	c.watch(c.mwc, fs.mwcInformer())
	c.watch(c.vwc, fs.vwcInformer())
}

func (c *Controller) wireIngress(runtime config.RuntimeConfig, fs factorySet) {
	if !runtime.IngressMonitor().Enabled {
		return
	}
	c.ingressLister = fs.ingressLister()
	c.watch(c.ingress, fs.ingressInformers()...)
}

func (c *Controller) wireNetpol(runtime config.RuntimeConfig, fs factorySet) {
	if !runtime.NetworkPolicyMonitor().Enabled {
		return
	}
	c.netpolLister = fs.netpolLister()
	c.watch(c.netpol, fs.netpolInformers()...)
}

func (c *Controller) wireClusterResources(
	runtime config.RuntimeConfig, fs factorySet,
) {
	if !runtime.ClusterResourceMonitor().Enabled {
		return
	}
	c.resourceQuotaLister = fs.resourceQuotaLister()
	c.watch(c.resourceQuota, fs.resourceQuotaInformers()...)
	c.limitRangeLister = fs.limitRangeLister()
	c.watch(c.limitRange, fs.limitRangeInformers()...)

	c.namespaceLister = fs.namespaceLister()
	if inf := fs.namespaceInformer(); inf != nil {
		c.watch(c.namespace, inf)
	}
	c.leaseLister = fs.leaseLister()
	c.watch(c.lease, fs.leaseInformers()...)
}

// wireControlPlane wires the kube-system pod informer and returns the dedicated
// factory it owns.
func (c *Controller) wireControlPlane(
	client kubernetes.Interface,
	resync time.Duration,
) informers.SharedInformerFactory {
	opts := append(
		[]informers.SharedInformerOption{informers.WithNamespace("kube-system")},
		informerMemoryOptions()...,
	)
	cpFactory := informers.NewSharedInformerFactoryWithOptions(
		client,
		resync,
		opts...,
	)
	cpPodInformer := cpFactory.Core().V1().Pods().Informer()

	c.cpPodLister = cpFactory.Core().V1().Pods().Lister()
	c.watch(c.cpPod, cpPodInformer)

	return cpFactory
}

// wireStatefulSet always wires the lister for graph support; queue handlers are
// only attached when the statefulset monitor is enabled.
func (c *Controller) wireStatefulSet(
	runtime config.RuntimeConfig, fs factorySet,
) {
	ssInformers := fs.ssInformers()

	c.ssLister = fs.ssLister()

	var ssSynced []cache.InformerSynced
	for _, inf := range ssInformers {
		ssSynced = append(ssSynced, inf.HasSynced)
	}
	c.ssSynced = ssSynced

	if runtime.StatefulSetMonitor().Enabled {
		c.listen(c.statefulSet, ssInformers...)
	}
}

// wirePDB wires the pdb monitor.
//
// Every informer is awaited, not just the first. With one factory per watched
// namespace, awaiting only the first meant baseline seeding ran against a
// partially populated cache, so PDBs in the other namespaces were not seeded
// and were re-announced as new after every restart.
func (c *Controller) wirePDB(runtime config.RuntimeConfig, fs factorySet) {
	if !runtime.PdbMonitor().Enabled {
		return
	}
	pdbInformers := fs.pdbInformers()
	if len(pdbInformers) == 0 {
		return
	}

	c.pdbLister = fs.pdbLister()
	for _, inf := range pdbInformers {
		c.pdb.synced = append(c.pdb.synced, inf.HasSynced)
	}

	c.listen(c.pdb, pdbInformers...)
}

// wireReplicaSet wires the replicaset lister used by owner resolution.
func (c *Controller) wireReplicaSet(
	runtime config.RuntimeConfig, fs factorySet,
) {
	c.rsLister = fs.rsLister()

	rsInformers := fs.rsInformers()
	var rsSynced []cache.InformerSynced
	for _, inf := range rsInformers {
		rsSynced = append(rsSynced, inf.HasSynced)
	}
	c.rsSynced = rsSynced
	if runtime.ClusterResourceMonitor().Enabled {
		c.watch(c.replicaSet, rsInformers...)
	}

}

// wireDaemonSetLister wires the daemonset lister used by owner resolution;
// queue handlers live in wireDaemonSetMonitor when the monitor is enabled.
func (c *Controller) wireDaemonSetLister(fs factorySet) {
	c.dsLister = fs.dsLister()

	var dsSynced []cache.InformerSynced
	for _, inf := range fs.dsInformers() {
		dsSynced = append(dsSynced, inf.HasSynced)
	}
	c.dsSynced = dsSynced

}
