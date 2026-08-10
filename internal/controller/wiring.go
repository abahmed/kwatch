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

	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/handler"
)

// wireNode sets up the node informer when either monitor is enabled.
func (c *Controller) wireNode(h handler.Handler, cfg *config.Config, fs factorySet) {
	if cfg.NodeMonitor.Enabled || cfg.NodeResourceMonitor.Enabled {
		nodeInformer := fs.nodeInformer()
		nodeLister := fs.nodeLister()

		c.nodeLister = nodeLister

		if cfg.NodeMonitor.Enabled {
			c.nodesSynced = nodeInformer.HasSynced
			h.SetNodeLister(nodeLister)
			nodeInformer.AddEventHandler(c.changeRecordingHandler("node", c.enqueueNode))
		}
	}
}

func (c *Controller) wireRollout(h handler.Handler, cfg *config.Config, fs factorySet) {
	if !cfg.RolloutMonitor.Enabled {
		return
	}
	deployLister := fs.deployLister()
	deployInformers := fs.deployInformers()

	c.deployLister = deployLister
	c.deploymentWatchEnabled = true

	var deploysSynced []cache.InformerSynced
	for _, inf := range deployInformers {
		deploysSynced = append(deploysSynced, inf.HasSynced)
	}
	c.deploysSynced = deploysSynced

	h.SetDeploymentLister(deployLister)

	for _, inf := range deployInformers {
		inf.AddEventHandler(c.changeRecordingHandler("deployment", c.enqueueDeployment))
	}
}

func (c *Controller) wireJobs(h handler.Handler, cfg *config.Config, fs factorySet) {
	if !cfg.JobMonitor.Enabled {
		return
	}
	jobLister := fs.jobLister()
	jobInformers := fs.jobInformers()

	c.jobLister = jobLister
	c.jobWatchEnabled = true

	var jobsSynced []cache.InformerSynced
	for _, inf := range jobInformers {
		jobsSynced = append(jobsSynced, inf.HasSynced)
	}
	c.jobsSynced = jobsSynced

	h.SetJobLister(jobLister)

	for _, inf := range jobInformers {
		inf.AddEventHandler(c.changeRecordingHandler("job", c.enqueueJob))
	}
}

func (c *Controller) wireDaemonSetMonitor(h handler.Handler, cfg *config.Config, fs factorySet) {
	if !cfg.DaemonSetMonitor.Enabled {
		return
	}
	dsLister := fs.dsLister()
	dsInformers := fs.dsInformers()

	c.daemonSetWatchEnabled = true

	var dssSynced []cache.InformerSynced
	for _, inf := range dsInformers {
		dssSynced = append(dssSynced, inf.HasSynced)
	}
	c.dsSynced = dssSynced

	h.SetDaemonSetLister(dsLister)

	for _, inf := range dsInformers {
		inf.AddEventHandler(c.changeRecordingHandler("daemonset", c.enqueueDaemonSet))
	}
}

func (c *Controller) wireCronJobs(h handler.Handler, cfg *config.Config, fs factorySet) {
	if cfg.CronJobMonitor.Enabled {
		c.cronJobLister = fs.cronJobLister()
		cronJobInformers := fs.cronJobInformers()

		c.cronJobWatchEnabled = true

		var cjSynced []cache.InformerSynced
		for _, inf := range cronJobInformers {
			cjSynced = append(cjSynced, inf.HasSynced)
		}
		c.cronJobsSynced = cjSynced

		h.SetCronJobLister(c.cronJobLister)

		for _, inf := range cronJobInformers {
			inf.AddEventHandler(c.changeRecordingHandler("cronjob", c.enqueueCronJob))
		}
	}
}

func (c *Controller) wireHPA(h handler.Handler, cfg *config.Config, fs factorySet) {
	if cfg.HpaMonitor.Enabled {
		c.hpaLister = fs.hpaLister()
		hpaInformers := fs.hpaInformers()

		c.hpaWatchEnabled = true

		var hpaSynced []cache.InformerSynced
		for _, inf := range hpaInformers {
			hpaSynced = append(hpaSynced, inf.HasSynced)
		}
		c.hpaSynced = hpaSynced

		h.SetHorizontalPodAutoscalerLister(c.hpaLister)

		for _, inf := range hpaInformers {
			inf.AddEventHandler(c.changeRecordingHandler("horizontalpodautoscaler", c.enqueueHorizontalPodAutoscaler))
		}
	}
}

func (c *Controller) wireService(h handler.Handler, cfg *config.Config, fs factorySet) {
	if cfg.ServiceMonitor.Enabled {
		svcLister := fs.serviceLister()
		svcInformers := fs.serviceInformers()
		epSliceLister := fs.endpointSliceLister()
		epSliceInformers := fs.endpointSliceInformers()

		c.serviceLister = svcLister
		c.endpointSliceLister = epSliceLister
		c.serviceWatchEnabled = true
		c.endpointSliceWatchEnabled = true

		var svcSynced, epSliceSynced []cache.InformerSynced
		for _, inf := range svcInformers {
			svcSynced = append(svcSynced, inf.HasSynced)
		}
		for _, inf := range epSliceInformers {
			epSliceSynced = append(epSliceSynced, inf.HasSynced)
		}
		c.svcSynced = svcSynced
		c.endpointSliceSynced = epSliceSynced

		h.SetServiceLister(svcLister)
		h.SetEndpointSliceLister(epSliceLister)

		for _, inf := range svcInformers {
			inf.AddEventHandler(c.changeRecordingHandler("service", c.enqueueService))
		}
		for _, inf := range epSliceInformers {
			inf.AddEventHandler(c.changeRecordingHandler("endpointslice", c.enqueueEndpointSlice))
		}
	}
}

func (c *Controller) wireAdmissionWebhooks(h handler.Handler, cfg *config.Config, fs factorySet) {
	if cfg.AdmissionWebhookMonitor.Enabled {
		mwcLister := fs.mwcLister()
		mwcInformer := fs.mwcInformer()
		vwcLister := fs.vwcLister()
		vwcInformer := fs.vwcInformer()

		c.mwcLister = mwcLister
		c.mwcSynced = mwcInformer.HasSynced
		c.vwcLister = vwcLister
		c.vwcSynced = vwcInformer.HasSynced
		c.mwcWatchEnabled = true
		c.vwcWatchEnabled = true

		h.SetMwCLister(mwcLister)
		h.SetVwCLister(vwcLister)

		mwcInformer.AddEventHandler(c.changeRecordingHandler("mutatingwebhookconfiguration", c.enqueueMwc))
		vwcInformer.AddEventHandler(c.changeRecordingHandler("validatingwebhookconfiguration", c.enqueueVwc))
	}
}

func (c *Controller) wireIngress(h handler.Handler, cfg *config.Config, fs factorySet) {
	if cfg.IngressMonitor.Enabled {
		ingressLister := fs.ingressLister()
		ingressInformers := fs.ingressInformers()

		c.ingressLister = ingressLister
		c.ingressWatchEnabled = true

		var ingressSynced []cache.InformerSynced
		for _, inf := range ingressInformers {
			ingressSynced = append(ingressSynced, inf.HasSynced)
		}
		c.ingressSynced = ingressSynced

		h.SetIngressLister(ingressLister)

		for _, inf := range ingressInformers {
			inf.AddEventHandler(c.changeRecordingHandler("ingress", c.enqueueIngress))
		}
	}
}

func (c *Controller) wireNetpol(h handler.Handler, cfg *config.Config, fs factorySet) {
	if cfg.NetworkPolicyMonitor.Enabled {
		netpolLister := fs.netpolLister()
		netpolInformers := fs.netpolInformers()

		c.netpolLister = netpolLister
		c.netpolWatchEnabled = true

		var netpolSynced []cache.InformerSynced
		for _, inf := range netpolInformers {
			netpolSynced = append(netpolSynced, inf.HasSynced)
		}
		c.netpolSynced = netpolSynced

		h.SetNetpolLister(netpolLister)

		for _, inf := range netpolInformers {
			inf.AddEventHandler(c.changeRecordingHandler("networkpolicy", c.enqueueNetpol))
		}
	}
}

// wireControlPlane wires the kube-system pod informer and returns the dedicated
// factory it owns.
func (c *Controller) wireControlPlane(h handler.Handler, client kubernetes.Interface, resync time.Duration) informers.SharedInformerFactory {
	cpFactory := informers.NewSharedInformerFactoryWithOptions(client, resync, informers.WithNamespace("kube-system"))
	cpPodInformer := cpFactory.Core().V1().Pods().Informer()
	cpPodLister := cpFactory.Core().V1().Pods().Lister()

	c.cpPodLister = cpPodLister
	c.cpSynced = cpPodInformer.HasSynced
	c.cpWatchEnabled = true

	h.SetCpPodLister(cpPodLister)

	cpPodInformer.AddEventHandler(c.changeRecordingHandler("pod", c.enqueueCpPod))
	return cpFactory
}

func (c *Controller) wireReplicaSet(h handler.Handler, fs factorySet) {
	c.rsLister = fs.rsLister()

	var rsSynced []cache.InformerSynced
	for _, inf := range fs.rsInformers() {
		rsSynced = append(rsSynced, inf.HasSynced)
	}
	c.rsSynced = rsSynced

	h.SetReplicaLister(c.rsLister)
}

func (c *Controller) wireDaemonSetLister(h handler.Handler, fs factorySet) {
	c.dsLister = fs.dsLister()

	var dsSynced []cache.InformerSynced
	for _, inf := range fs.dsInformers() {
		dsSynced = append(dsSynced, inf.HasSynced)
	}
	c.dsSynced = dsSynced

	h.SetDaemonSetLister(c.dsLister)
}

func (c *Controller) wireStatefulSet(h handler.Handler, cfg *config.Config, fs factorySet) {
	c.ssLister = fs.ssLister()

	var ssSynced []cache.InformerSynced
	for _, inf := range fs.ssInformers() {
		ssSynced = append(ssSynced, inf.HasSynced)
	}
	c.ssSynced = ssSynced

	h.SetStatefulSetLister(c.ssLister)

	if cfg.StatefulSetMonitor.Enabled {
		c.statefulSetWatchEnabled = true
		for _, inf := range fs.ssInformers() {
			inf.AddEventHandler(c.changeRecordingHandler("statefulset", c.enqueueStatefulSet))
		}
	}
}

func (c *Controller) wirePDB(h handler.Handler, cfg *config.Config, fs factorySet) {
	if cfg.PdbMonitor.Enabled {
		c.pdbLister = fs.pdbLister()
		pdbInformers := fs.pdbInformers()

		c.pdbWatchEnabled = true
		c.pdbSynced = pdbInformers[0].HasSynced

		h.SetPdbLister(c.pdbLister)

		for _, inf := range pdbInformers {
			inf.AddEventHandler(c.changeRecordingHandler("poddisruptionbudget", c.enqueuePdb))
		}
	}
}

// wireEvents wires the pod-event informers. It returns the additional factory
// instances (one per monitored namespace) that must be started.
func (c *Controller) wireEvents(h handler.Handler, client kubernetes.Interface, resync time.Duration, namespaces []string) []informers.SharedInformerFactory {
	var eventFactories []informers.SharedInformerFactory
	if len(namespaces) <= 1 {
		opts := []informers.SharedInformerOption{
			informers.WithTweakListOptions(func(o *metav1.ListOptions) {
				o.FieldSelector = "involvedObject.kind=Pod"
			}),
		}
		if len(namespaces) == 1 {
			opts = append(opts, informers.WithNamespace(namespaces[0]))
		}
		ef := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
		eventFactories = append(eventFactories, ef)
		eventInformer := ef.Core().V1().Events().Informer()
		utilruntime.Must(eventInformer.AddIndexers(cache.Indexers{
			"byPod": func(obj interface{}) ([]string, error) {
				if ev, ok := obj.(*corev1.Event); ok {
					return []string{ev.InvolvedObject.Name}, nil
				}
				return nil, nil
			},
		}))
		c.eventLister = ef.Core().V1().Events().Lister()
		c.eventsSynced = append(c.eventsSynced, eventInformer.HasSynced)
	} else {
		listers := make([]corev1lister.EventLister, 0, len(namespaces))
		for _, ns := range namespaces {
			ns := ns
			opts := []informers.SharedInformerOption{
				informers.WithTweakListOptions(func(o *metav1.ListOptions) {
					o.FieldSelector = "involvedObject.kind=Pod"
				}),
				informers.WithNamespace(ns),
			}
			ef := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
			eventFactories = append(eventFactories, ef)
			eventInformer := ef.Core().V1().Events().Informer()
			utilruntime.Must(eventInformer.AddIndexers(cache.Indexers{
				"byPod": func(obj interface{}) ([]string, error) {
					if ev, ok := obj.(*corev1.Event); ok {
						return []string{ev.InvolvedObject.Name}, nil
					}
					return nil, nil
				},
			}))
			listers = append(listers, ef.Core().V1().Events().Lister())
			c.eventsSynced = append(c.eventsSynced, eventInformer.HasSynced)
		}
		c.eventLister = &multiEventLister{listers: listers}
	}
	h.SetEventLister(c.eventLister)
	return eventFactories
}

// wireClusterAutoscaler wires the cluster-autoscaler event informer and returns
// its dedicated factory.
func wireClusterAutoscaler(h handler.Handler, client kubernetes.Interface, resync time.Duration) informers.SharedInformerFactory {
	caOpts := []informers.SharedInformerOption{
		informers.WithTweakListOptions(func(o *metav1.ListOptions) {
			o.FieldSelector = "source=cluster-autoscaler"
		}),
		informers.WithNamespace("kube-system"),
	}
	caFactory := informers.NewSharedInformerFactoryWithOptions(client, resync, caOpts...)
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
}

// wireTLS wires the TLS secret informer and returns the factories it creates.
func (c *Controller) wireTLS(h handler.Handler, client kubernetes.Interface, resync time.Duration, namespaces []string) []informers.SharedInformerFactory {
	var tlsFactories []informers.SharedInformerFactory
	if len(namespaces) <= 1 {
		opts := []informers.SharedInformerOption{
			informers.WithTweakListOptions(func(o *metav1.ListOptions) {
				o.FieldSelector = "type=kubernetes.io/tls"
			}),
		}
		if len(namespaces) == 1 {
			opts = append(opts, informers.WithNamespace(namespaces[0]))
		}
		tf := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
		tlsFactories = append(tlsFactories, tf)
		c.secretLister = tf.Core().V1().Secrets().Lister()
		c.secretsSynced = append(c.secretsSynced, tf.Core().V1().Secrets().Informer().HasSynced)
	} else {
		listers := make([]corev1lister.SecretLister, 0, len(namespaces))
		for _, ns := range namespaces {
			ns := ns
			opts := []informers.SharedInformerOption{
				informers.WithTweakListOptions(func(o *metav1.ListOptions) {
					o.FieldSelector = "type=kubernetes.io/tls"
				}),
				informers.WithNamespace(ns),
			}
			tf := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
			tlsFactories = append(tlsFactories, tf)
			listers = append(listers, tf.Core().V1().Secrets().Lister())
			c.secretsSynced = append(c.secretsSynced, tf.Core().V1().Secrets().Informer().HasSynced)
		}
		c.secretLister = &multiSecretLister{listers: listers}
	}
	h.SetSecretLister(c.secretLister)
	return tlsFactories
}
