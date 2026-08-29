package controller

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/handler"
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
	if !cfg.ServiceMonitor.Enabled {
		return
	}
	c.serviceLister = fs.serviceLister()
	c.endpointSliceLister = fs.endpointSliceLister()

	c.watch(c.service, fs.serviceInformers()...)
	c.watch(c.endpointSlice, fs.endpointSliceInformers()...)
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

// wireEvents wires the pod-event informers. It returns the additional factory
// instances (one per monitored namespace) that must be started.
func (c *Controller) wireEvents(
	client kubernetes.Interface,
	resync time.Duration,
	scope namespaceScope,
) []informers.SharedInformerFactory {
	var eventFactories []informers.SharedInformerFactory
	if !scope.all && len(scope.namespaces) == 0 {
		return eventFactories
	}
	if scope.all || len(scope.namespaces) == 1 {
		opts := []informers.SharedInformerOption{
			informers.WithTweakListOptions(func(o *metav1.ListOptions) {
				o.FieldSelector = "involvedObject.kind=Pod"
				if scope.all {
					o.FieldSelector += "," + informerExcludedNamespaces(scope.forbidden)
				}
			}),
		}
		if len(scope.namespaces) == 1 {
			opts = append(opts, informers.WithNamespace(scope.namespaces[0]))
		}
		ef := informers.NewSharedInformerFactoryWithOptions(
			client,
			resync,
			opts...)
		eventFactories = append(eventFactories, ef)
		eventInformer := ef.Core().V1().Events().Informer()
		utilruntime.Must(eventInformer.AddIndexers(cache.Indexers{
			"byPod": func(obj interface{}) ([]string, error) {
				if ev, ok := obj.(*corev1.Event); ok {
					return []string{
						eventsByPodKey(
							ev.InvolvedObject.Namespace,
							ev.InvolvedObject.Name,
						),
					}, nil
				}
				return nil, nil
			},
		}))
		c.eventLister = ef.Core().V1().Events().Lister()
		c.eventIndexers = append(c.eventIndexers, eventInformer.GetIndexer())
		c.eventsSynced = append(c.eventsSynced, eventInformer.HasSynced)
	} else {
		listers := make([]corev1lister.EventLister, 0, len(scope.namespaces))
		for _, ns := range scope.namespaces {
			ns := ns
			opts := []informers.SharedInformerOption{
				informers.WithTweakListOptions(func(o *metav1.ListOptions) {
					o.FieldSelector = "involvedObject.kind=Pod"
				}),
				informers.WithNamespace(ns),
			}
			ef := informers.NewSharedInformerFactoryWithOptions(
				client,
				resync,
				opts...)
			eventFactories = append(eventFactories, ef)
			eventInformer := ef.Core().V1().Events().Informer()
			utilruntime.Must(eventInformer.AddIndexers(cache.Indexers{
				"byPod": func(obj interface{}) ([]string, error) {
					if ev, ok := obj.(*corev1.Event); ok {
						return []string{
							eventsByPodKey(
								ev.InvolvedObject.Namespace,
								ev.InvolvedObject.Name,
							),
						}, nil
					}
					return nil, nil
				},
			}))
			listers = append(listers, ef.Core().V1().Events().Lister())
			c.eventIndexers = append(
				c.eventIndexers,
				eventInformer.GetIndexer(),
			)
			c.eventsSynced = append(c.eventsSynced, eventInformer.HasSynced)
		}
		c.eventLister = &multiEventLister{listers: listers}
	}
	return eventFactories
}

// eventsByPodIndex is the informer index that maps "namespace/pod" to the
// events involving that pod.
const eventsByPodIndex = "byPod"

func eventsByPodKey(namespace, pod string) string {
	return namespace + "/" + pod
}

// eventsByPod resolves a pod's events straight from the informer index. With
// several namespaced informers each holds a disjoint slice of the cluster, so
// asking all of them is still one indexed lookup each rather than a scan.
func (c *Controller) eventsByPod(
	namespace, pod string,
) ([]*corev1.Event, error) {
	var out []*corev1.Event
	for _, ix := range c.eventIndexers {
		objs, err := ix.ByIndex(
			eventsByPodIndex,
			eventsByPodKey(namespace, pod),
		)
		if err != nil {
			return nil, err
		}
		for _, o := range objs {
			if ev, ok := o.(*corev1.Event); ok {
				out = append(out, ev)
			}
		}
	}
	return out, nil
}

// wireClusterAutoscaler wires the cluster-autoscaler event informer and returns
// its dedicated factory.
func wireClusterAutoscaler(
	h handler.Handler,
	client kubernetes.Interface,
	resync time.Duration,
) informers.SharedInformerFactory {
	caOpts := []informers.SharedInformerOption{
		informers.WithTweakListOptions(func(o *metav1.ListOptions) {
			o.FieldSelector = "source=cluster-autoscaler"
		}),
		informers.WithNamespace("kube-system"),
	}
	caFactory := informers.NewSharedInformerFactoryWithOptions(
		client,
		resync,
		caOpts...)
	caEventInformer := caFactory.Core().V1().Events().Informer()
	utilruntime.Must(caEventInformer.AddIndexers(cache.Indexers{
		"byReason": func(obj interface{}) ([]string, error) {
			if ev, ok := obj.(*corev1.Event); ok {
				return []string{ev.Reason}, nil
			}
			return nil, nil
		},
	}))
	caEventInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if ev, ok := obj.(*corev1.Event); ok {
				h.ProcessClusterAutoscalerEvent(ev)
			}
		},
		UpdateFunc: func(_, obj interface{}) {
			if ev, ok := obj.(*corev1.Event); ok {
				h.ProcessClusterAutoscalerEvent(ev)
			}
		},
	})
	return caFactory
}

// wireConfigMap wires the shared configmap informer used for dependency
// tracking. It records changes on every monitored namespace.
func (c *Controller) wireConfigMap(fs factorySet) {
	cmLister := fs.configMapLister()
	cmInformers := fs.configMapInformers()

	c.configMapLister = cmLister
	for _, inf := range cmInformers {
		c.configMapSynced = append(c.configMapSynced, inf.HasSynced)
		inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				c.recordChange(kwcontext.ChangeCreate, "configmap", obj)
			},
			UpdateFunc: func(old, new interface{}) {
				c.recordChange(kwcontext.ChangeUpdate, "configmap", new)
			},
			DeleteFunc: func(obj interface{}) {
				c.recordChange(kwcontext.ChangeDelete, "configmap", obj)
				if c.graph != nil {
					if cm, ok := obj.(*corev1.ConfigMap); ok {
						c.graph.RemoveNode("configmap", cm.Namespace, cm.Name)
					}
				}
			},
		})
	}
}

// wireGraphSupport assigns the lister extras used by the resource graph.
func (c *Controller) wireGraphSupport(fs factorySet) {
	c.pvcLister = fs.pvcLister()
	c.pvLister = fs.persistentVolumeLister()
	c.serviceAccountLister = fs.serviceAccountLister()
	c.storageClassLister = fs.storageClassLister()
	for _, inf := range fs.pvcInformers() {
		c.graphSynced = append(c.graphSynced, inf.HasSynced)
	}
	for _, inf := range fs.serviceAccountInformers() {
		c.graphSynced = append(c.graphSynced, inf.HasSynced)
	}
	for _, inf := range fs.persistentVolumeInformers() {
		c.graphSynced = append(c.graphSynced, inf.HasSynced)
	}
	for _, inf := range fs.storageClassInformers() {
		c.graphSynced = append(c.graphSynced, inf.HasSynced)
	}
	// Cluster-scoped listers only exist when a global/cluster factory was
	// created; watching multiple namespaces skips PV and storage class edges.
	if c.pvLister == nil || c.storageClassLister == nil {
		klog.InfoS(
			"multi-namespace watch: persistentvolume and storageclass graph " +
				"edges are unavailable",
		)
	}
}

// wireTLS wires the TLS secret informer and returns the factories it creates.
func (c *Controller) wireTLS(
	client kubernetes.Interface,
	resync time.Duration,
	scope namespaceScope,
) []informers.SharedInformerFactory {
	var tlsFactories []informers.SharedInformerFactory
	if !scope.all && len(scope.namespaces) == 0 {
		return tlsFactories
	}
	if scope.all || len(scope.namespaces) == 1 {
		opts := []informers.SharedInformerOption{
			informers.WithTweakListOptions(func(o *metav1.ListOptions) {
				o.FieldSelector = "type=kubernetes.io/tls"
				if scope.all {
					o.FieldSelector += "," + informerExcludedNamespaces(scope.forbidden)
				}
			}),
		}
		if len(scope.namespaces) == 1 {
			opts = append(opts, informers.WithNamespace(scope.namespaces[0]))
		}
		tf := informers.NewSharedInformerFactoryWithOptions(
			client,
			resync,
			opts...)
		tlsFactories = append(tlsFactories, tf)
		c.secretLister = tf.Core().V1().Secrets().Lister()
		c.secretsSynced = append(
			c.secretsSynced,
			tf.Core().V1().Secrets().Informer().HasSynced,
		)
	} else {
		listers := make([]corev1lister.SecretLister, 0, len(scope.namespaces))
		for _, ns := range scope.namespaces {
			ns := ns
			opts := []informers.SharedInformerOption{
				informers.WithTweakListOptions(func(o *metav1.ListOptions) {
					o.FieldSelector = "type=kubernetes.io/tls"
				}),
				informers.WithNamespace(ns),
			}
			tf := informers.NewSharedInformerFactoryWithOptions(
				client,
				resync,
				opts...)
			tlsFactories = append(tlsFactories, tf)
			listers = append(listers, tf.Core().V1().Secrets().Lister())
			c.secretsSynced = append(
				c.secretsSynced,
				tf.Core().V1().Secrets().Informer().HasSynced,
			)
		}
		c.secretLister = &multiSecretLister{listers: listers}
	}
	return tlsFactories
}
