package controller

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/context"
)

// wireNode sets up the node informer when either monitor is enabled.
func (c *Controller) wireNode(cfg *config.Config, fs factorySet) {
	if cfg.NodeMonitor.Enabled || cfg.NodeResourceMonitor.Enabled {
		c.nodeLister = fs.nodeLister()

		if cfg.NodeMonitor.Enabled {
			c.watch(c.node, fs.nodeInformer())
		}
	}
}

func (c *Controller) wireRollout(cfg *config.Config, fs factorySet) {
	if !cfg.RolloutMonitor.Enabled {
		return
	}
	c.deployLister = fs.deployLister()
	c.watch(c.deployment, fs.deployInformers()...)
}

func (c *Controller) wireJobs(cfg *config.Config, fs factorySet) {
	if !cfg.JobMonitor.Enabled {
		return
	}
	c.jobLister = fs.jobLister()
	c.watch(c.job, fs.jobInformers()...)
}

func (c *Controller) wireDaemonSetMonitor(cfg *config.Config, fs factorySet) {
	if !cfg.DaemonSetMonitor.Enabled {
		return
	}
	c.watch(c.daemonSet, fs.dsInformers()...)
}

func (c *Controller) wireCronJobs(cfg *config.Config, fs factorySet) {
	if !cfg.CronJobMonitor.Enabled {
		return
	}
	c.cronJobLister = fs.cronJobLister()
	c.watch(c.cronJob, fs.cronJobInformers()...)
}

func (c *Controller) wireHPA(cfg *config.Config, fs factorySet) {
	if !cfg.HpaMonitor.Enabled {
		return
	}
	c.hpaLister = fs.hpaLister()
	c.watch(c.hpa, fs.hpaInformers()...)
}

func (c *Controller) wireService(cfg *config.Config, fs factorySet) {
	if !cfg.ServiceMonitor.Enabled && !cfg.IngressMonitor.Enabled &&
		!cfg.AdmissionWebhookMonitor.Enabled {
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
				if cfg.ServiceMonitor.Enabled {
					c.service.enqueue(obj)
				}
				c.enqueueServiceDependents()
			},
			UpdateFunc: func(_, obj interface{}) {
				c.recordChange(kwcontext.ChangeUpdate, "service", obj)
				if cfg.ServiceMonitor.Enabled {
					c.service.enqueue(obj)
				}
				c.enqueueServiceDependents()
			},
			DeleteFunc: func(obj interface{}) {
				c.recordChange(kwcontext.ChangeDelete, "service", obj)
				if cfg.ServiceMonitor.Enabled {
					c.service.enqueue(obj)
				}
				c.enqueueServiceDependents()
			},
		})
	}
	if !cfg.ServiceMonitor.Enabled {
		return
	}
	c.endpointSliceLister = fs.endpointSliceLister()
	c.service.startWorkers = true
	c.watch(c.endpointSlice, fs.endpointSliceInformers()...)
}

// enqueueServiceDependents rechecks objects whose detector reads the Service
// lister. A Service change can resolve an Ingress or admission-webhook issue
// without changing the referencing object itself.
func (c *Controller) enqueueServiceDependents() {
	if c.ingress.startWorkers && c.ingressLister != nil {
		if items, err := c.ingressLister.Ingresses(metav1.NamespaceAll).List(labels.Everything()); err == nil {
			for _, item := range items {
				c.ingress.enqueue(item)
			}
		}
	}
	if c.mwc.startWorkers && c.mwcLister != nil {
		if items, err := c.mwcLister.List(labels.Everything()); err == nil {
			for _, item := range items {
				c.mwc.enqueue(item)
			}
		}
	}
	if c.vwc.startWorkers && c.vwcLister != nil {
		if items, err := c.vwcLister.List(labels.Everything()); err == nil {
			for _, item := range items {
				c.vwc.enqueue(item)
			}
		}
	}
}

func (c *Controller) wireAdmissionWebhooks(cfg *config.Config, fs factorySet) {
	if !cfg.AdmissionWebhookMonitor.Enabled {
		return
	}
	mwcLister := fs.mwcLister()
	vwcLister := fs.vwcLister()

	c.mwcLister = mwcLister
	c.vwcLister = vwcLister

	c.watch(c.mwc, fs.mwcInformer())
	c.watch(c.vwc, fs.vwcInformer())
}

func (c *Controller) wireIngress(cfg *config.Config, fs factorySet) {
	if !cfg.IngressMonitor.Enabled {
		return
	}
	c.ingressLister = fs.ingressLister()
	c.watch(c.ingress, fs.ingressInformers()...)
}

func (c *Controller) wireNetpol(cfg *config.Config, fs factorySet) {
	if !cfg.NetworkPolicyMonitor.Enabled {
		return
	}
	c.netpolLister = fs.netpolLister()
	c.watch(c.netpol, fs.netpolInformers()...)
}

// wireControlPlane wires the kube-system pod informer and returns the dedicated
// factory it owns.
func (c *Controller) wireControlPlane(
	client kubernetes.Interface,
	resync time.Duration,
) informers.SharedInformerFactory {
	cpFactory := informers.NewSharedInformerFactoryWithOptions(
		client,
		resync,
		informers.WithNamespace("kube-system"),
	)
	cpPodInformer := cpFactory.Core().V1().Pods().Informer()

	c.cpPodLister = cpFactory.Core().V1().Pods().Lister()
	c.watch(c.cpPod, cpPodInformer)

	return cpFactory
}

// wireStatefulSet always wires the lister for graph support; queue handlers are
// only attached when the statefulset monitor is enabled.
func (c *Controller) wireStatefulSet(cfg *config.Config, fs factorySet) {
	ssInformers := fs.ssInformers()

	c.ssLister = fs.ssLister()

	var ssSynced []cache.InformerSynced
	for _, inf := range ssInformers {
		ssSynced = append(ssSynced, inf.HasSynced)
	}
	c.ssSynced = ssSynced

	if cfg.StatefulSetMonitor.Enabled {
		c.listen(c.statefulSet, ssInformers...)
	}
}

// wirePDB wires the pdb monitor. Only the first informer's HasSynced is
// awaited, matching the historical single-sync behavior.
func (c *Controller) wirePDB(cfg *config.Config, fs factorySet) {
	if !cfg.PdbMonitor.Enabled {
		return
	}
	pdbInformers := fs.pdbInformers()
	if len(pdbInformers) == 0 {
		return
	}

	c.pdbLister = fs.pdbLister()
	c.pdb.synced = []cache.InformerSynced{pdbInformers[0].HasSynced}

	c.listen(c.pdb, pdbInformers...)
}

// wireReplicaSet wires the replicaset lister used by owner resolution.
func (c *Controller) wireReplicaSet(fs factorySet) {
	c.rsLister = fs.rsLister()

	var rsSynced []cache.InformerSynced
	for _, inf := range fs.rsInformers() {
		rsSynced = append(rsSynced, inf.HasSynced)
	}
	c.rsSynced = rsSynced

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
