package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	admissionregistrationv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/resource"
)

type Controller struct {
	handler                 handler.Handler
	podQueue                workqueue.TypedRateLimitingInterface[string]
	nodeQueue               workqueue.TypedRateLimitingInterface[string]
	deploymentQueue         workqueue.TypedRateLimitingInterface[string]
	jobQueue                workqueue.TypedRateLimitingInterface[string]
	daemonSetQueue          workqueue.TypedRateLimitingInterface[string]
	statefulSetQueue        workqueue.TypedRateLimitingInterface[string]
	pdbQueue                workqueue.TypedRateLimitingInterface[string]
	cronJobQueue            workqueue.TypedRateLimitingInterface[string]
	podLister               corev1lister.PodLister
	podsSynced              []cache.InformerSynced
	nodeLister              corev1lister.NodeLister
	nodesSynced             cache.InformerSynced
	deployLister            appsv1lister.DeploymentLister
	deploysSynced           []cache.InformerSynced
	jobLister               batchv1lister.JobLister
	jobsSynced              []cache.InformerSynced
	cronJobLister           batchv1lister.CronJobLister
	cronJobsSynced          []cache.InformerSynced
	rsLister                appsv1lister.ReplicaSetLister
	rsSynced                []cache.InformerSynced
	dsLister                appsv1lister.DaemonSetLister
	dsSynced                []cache.InformerSynced
	ssLister                appsv1lister.StatefulSetLister
	ssSynced                []cache.InformerSynced
	pdbLister               policyv1lister.PodDisruptionBudgetLister
	pdbSynced               cache.InformerSynced
	eventLister             corev1lister.EventLister
	eventsSynced            []cache.InformerSynced
	deploymentWatchEnabled  bool
	jobWatchEnabled         bool
	daemonSetWatchEnabled   bool
	statefulSetWatchEnabled bool
	pdbWatchEnabled         bool
	cronJobWatchEnabled     bool
	hpaQueue                workqueue.TypedRateLimitingInterface[string]
	hpaLister               autoscalingv2lister.HorizontalPodAutoscalerLister
	hpaSynced               []cache.InformerSynced
	hpaWatchEnabled         bool
	secretLister            corev1lister.SecretLister
	secretsSynced           []cache.InformerSynced
	maxBaseline             int

	tracker         *kwcontext.ChangeTracker
	graph           *kwcontext.ResourceGraph
	configMapLister corev1lister.ConfigMapLister
	configMapSynced []cache.InformerSynced

	serviceQueue              workqueue.TypedRateLimitingInterface[string]
	endpointSliceQueue        workqueue.TypedRateLimitingInterface[string]
	mwcQueue                  workqueue.TypedRateLimitingInterface[string]
	vwcQueue                  workqueue.TypedRateLimitingInterface[string]
	ingressQueue              workqueue.TypedRateLimitingInterface[string]
	netpolQueue               workqueue.TypedRateLimitingInterface[string]
	cpPodQueue                workqueue.TypedRateLimitingInterface[string]
	serviceLister             corev1lister.ServiceLister
	svcSynced                 []cache.InformerSynced
	endpointSliceLister       discoveryv1lister.EndpointSliceLister
	endpointSliceSynced       []cache.InformerSynced
	mwcLister                 admissionregistrationv1lister.MutatingWebhookConfigurationLister
	mwcSynced                 cache.InformerSynced
	vwcLister                 admissionregistrationv1lister.ValidatingWebhookConfigurationLister
	vwcSynced                 cache.InformerSynced
	ingressLister             networkingv1lister.IngressLister
	ingressSynced             []cache.InformerSynced
	netpolLister              networkingv1lister.NetworkPolicyLister
	netpolSynced              []cache.InformerSynced
	cpPodLister               corev1lister.PodLister
	cpSynced                  cache.InformerSynced
	serviceWatchEnabled       bool
	endpointSliceWatchEnabled bool
	mwcWatchEnabled           bool
	vwcWatchEnabled           bool
	ingressWatchEnabled       bool
	netpolWatchEnabled        bool
	cpWatchEnabled            bool

	nodeResourceCfg *config.NodeResourceMonitor

	readyFn func()
}

// resolveNamespaces decides which namespaces to watch.
// If NamespaceSelector is set, it lists namespaces via k8s API using the label
// selector. Otherwise it uses the static AllowedNamespaces/ForbiddenNamespaces.

func New(
	client kubernetes.Interface,
	cfg *config.Config,
	h handler.Handler,
) (*Controller, func()) {
	resync := time.Duration(cfg.ResyncSeconds) * time.Second

	namespaces, err := resolveNamespaces(cfg, client)
	if err != nil {
		klog.ErrorS(err, "failed to resolve namespaces")
		os.Exit(1)
	}

	fs, factories := newFactories(client, namespaces, resync)

	podLister := fs.podLister()
	podInformers := fs.podInformers()

	var podsSynced []cache.InformerSynced
	for _, inf := range podInformers {
		podsSynced = append(podsSynced, inf.HasSynced)
	}

	maxBaseline := cfg.Correlation.MaxBaseline
	if maxBaseline <= 0 {
		maxBaseline = correlation.DefaultMaxBaseline
	}

	c := &Controller{
		handler:            h,
		podQueue:           workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "pods"}),
		nodeQueue:          workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "nodes"}),
		deploymentQueue:    workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "deployments"}),
		jobQueue:           workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "jobs"}),
		daemonSetQueue:     workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "daemonsets"}),
		statefulSetQueue:   workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "statefulsets"}),
		pdbQueue:           workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "poddisruptionbudgets"}),
		cronJobQueue:       workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "cronjobs"}),
		hpaQueue:           workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "horizontalpodautoscalers"}),
		serviceQueue:       workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "services"}),
		endpointSliceQueue: workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "endpointslices"}),
		mwcQueue:           workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "mutatingwebhookconfigurations"}),
		vwcQueue:           workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "validatingwebhookconfigurations"}),
		ingressQueue:       workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "ingresses"}),
		netpolQueue:        workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "networkpolicies"}),
		cpPodQueue:         workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "controlplanepods"}),
		podLister:          podLister,
		podsSynced:         podsSynced,
		maxBaseline:        maxBaseline,
	}

	h.SetPodLister(podLister)

	for _, inf := range podInformers {
		inf.AddEventHandler(c.podEventHandler())
	}

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

	if cfg.RolloutMonitor.Enabled {
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

	if cfg.JobMonitor.Enabled {
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

	if cfg.DaemonSetMonitor.Enabled {
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

	if cfg.ControlPlaneMonitor.Enabled {
		cpFactory := informers.NewSharedInformerFactoryWithOptions(client, resync, informers.WithNamespace("kube-system"))
		cpPodInformer := cpFactory.Core().V1().Pods().Informer()
		cpPodLister := cpFactory.Core().V1().Pods().Lister()

		c.cpPodLister = cpPodLister
		c.cpSynced = cpPodInformer.HasSynced
		c.cpWatchEnabled = true

		h.SetCpPodLister(cpPodLister)

		cpPodInformer.AddEventHandler(c.changeRecordingHandler("pod", c.enqueueCpPod))

		factories = append(factories, cpFactory)
	}

	{
		c.rsLister = fs.rsLister()

		var rsSynced []cache.InformerSynced
		for _, inf := range fs.rsInformers() {
			rsSynced = append(rsSynced, inf.HasSynced)
		}
		c.rsSynced = rsSynced

		h.SetReplicaLister(c.rsLister)
	}

	{
		c.dsLister = fs.dsLister()

		var dsSynced []cache.InformerSynced
		for _, inf := range fs.dsInformers() {
			dsSynced = append(dsSynced, inf.HasSynced)
		}
		c.dsSynced = dsSynced

		h.SetDaemonSetLister(c.dsLister)
	}

	{
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

	// Events informer uses a dedicated factory with field selector to only cache Pod events
	{
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
					ev, ok := obj.(*corev1.Event)
					if !ok {
						return nil, nil
					}
					return []string{ev.InvolvedObject.Name}, nil
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
						ev, ok := obj.(*corev1.Event)
						if !ok {
							return nil, nil
						}
						return []string{ev.InvolvedObject.Name}, nil
					},
				}))
				listers = append(listers, ef.Core().V1().Events().Lister())
				c.eventsSynced = append(c.eventsSynced, eventInformer.HasSynced)
			}
			c.eventLister = &multiEventLister{listers: listers}
		}
		h.SetEventLister(c.eventLister)
		factories = append(factories, eventFactories...)
	}

	if cfg.ClusterAutoscalerMonitor.Enabled {
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
				ev, ok := obj.(*corev1.Event)
				if !ok {
					return nil, nil
				}
				return []string{ev.Reason}, nil
			},
		}))
		c.eventsSynced = append(c.eventsSynced, caEventInformer.HasSynced)
		factories = append(factories, caFactory)

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
	}

	// ConfigMap informer — always watches monitored namespaces for dependency tracking
	{
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

	if cfg.TlsMonitor.Enabled {
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
		factories = append(factories, tlsFactories...)
	}

	stopCh := make(chan struct{})
	for _, f := range factories {
		f.Start(stopCh)
	}

	if cfg.NodeResourceMonitor.Enabled {
		nc := cfg.NodeResourceMonitor
		c.nodeResourceCfg = &nc
	}

	cleanup := func() {
		close(stopCh)
		for _, f := range factories {
			f.Shutdown()
		}
	}

	return c, cleanup
}

func (c *Controller) SetReadyFunc(fn func()) { c.readyFn = fn }

func (c *Controller) SetTracker(t *kwcontext.ChangeTracker) { c.tracker = t }
func (c *Controller) SetGraph(g *kwcontext.ResourceGraph)   { c.graph = g }

func (c *Controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.podQueue.ShutDown()
	defer c.nodeQueue.ShutDown()
	defer c.deploymentQueue.ShutDown()
	defer c.jobQueue.ShutDown()
	defer c.daemonSetQueue.ShutDown()
	defer c.statefulSetQueue.ShutDown()
	defer c.pdbQueue.ShutDown()
	defer c.cronJobQueue.ShutDown()
	defer c.hpaQueue.ShutDown()
	defer c.serviceQueue.ShutDown()
	defer c.endpointSliceQueue.ShutDown()
	defer c.mwcQueue.ShutDown()
	defer c.vwcQueue.ShutDown()
	defer c.ingressQueue.ShutDown()
	defer c.netpolQueue.ShutDown()
	defer c.cpPodQueue.ShutDown()

	klog.InfoS("starting controller")

	klog.InfoS("waiting for informer caches to sync")
	syncFns := make([]cache.InformerSynced, 0,
		1+len(c.podsSynced)+len(c.rsSynced)+len(c.dsSynced)+len(c.ssSynced)+len(c.eventsSynced)+
			len(c.deploysSynced)+len(c.jobsSynced)+len(c.cronJobsSynced)+len(c.secretsSynced)+
			len(c.svcSynced)+len(c.endpointSliceSynced)+len(c.ingressSynced)+len(c.netpolSynced)+3)
	syncFns = append(syncFns, c.podsSynced...)
	syncFns = append(syncFns, c.rsSynced...)
	syncFns = append(syncFns, c.dsSynced...)
	syncFns = append(syncFns, c.ssSynced...)
	syncFns = append(syncFns, c.eventsSynced...)
	if c.nodesSynced != nil {
		syncFns = append(syncFns, c.nodesSynced)
	}
	syncFns = append(syncFns, c.configMapSynced...)
	syncFns = append(syncFns, c.deploysSynced...)
	syncFns = append(syncFns, c.jobsSynced...)
	syncFns = append(syncFns, c.cronJobsSynced...)
	syncFns = append(syncFns, c.hpaSynced...)
	syncFns = append(syncFns, c.secretsSynced...)
	syncFns = append(syncFns, c.svcSynced...)
	syncFns = append(syncFns, c.endpointSliceSynced...)
	if c.mwcSynced != nil {
		syncFns = append(syncFns, c.mwcSynced)
	}
	if c.vwcSynced != nil {
		syncFns = append(syncFns, c.vwcSynced)
	}
	syncFns = append(syncFns, c.ingressSynced...)
	syncFns = append(syncFns, c.netpolSynced...)
	if c.cpSynced != nil {
		syncFns = append(syncFns, c.cpSynced)
	}
	if c.pdbSynced != nil {
		syncFns = append(syncFns, c.pdbSynced)
	}
	if !cache.WaitForCacheSync(ctx.Done(), syncFns...) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	if c.readyFn != nil {
		c.readyFn()
	}

	c.buildGraph()
	go func() {
		rebuildTicker := time.NewTicker(60 * time.Minute)
		defer rebuildTicker.Stop()
		pruneTicker := time.NewTicker(5 * time.Minute)
		defer pruneTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-rebuildTicker.C:
				c.buildGraph()
			case <-pruneTicker.C:
				c.pruneGraph()
			}
		}
	}()
	c.buildSeenSet()
	if c.cpWatchEnabled {
		c.handler.SweepControlPlane()
	}

	if c.nodeResourceCfg != nil {
		go func(cfg *config.NodeResourceMonitor) {
			interval := time.Duration(cfg.IntervalSeconds) * time.Second
			if interval <= 0 {
				interval = 300 * time.Second
			}
			mon := resource.NewMonitor(resource.Config{
				Interval:   interval,
				CpuWarning: cfg.CpuWarning, CpuCritical: cfg.CpuCritical,
				MemWarning: cfg.MemWarning, MemCritical: cfg.MemCritical,
			}, c.nodeLister, c.podLister)
			mon.Run(ctx, func(sig *event.Signal) {
				c.handler.ProcessNodeResourceOvercommit(sig.Reason, sig.NodeName, sig.Hint, sig.Severity)
			})
		}(c.nodeResourceCfg)
	}

	klog.InfoS("starting workers")
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runPodWorker, time.Second)
		if c.nodesSynced != nil {
			go wait.UntilWithContext(ctx, c.runNodeWorker, time.Second)
		}
		if c.deploysSynced != nil {
			go wait.UntilWithContext(ctx, c.runDeploymentWorker, time.Second)
		}
		if c.jobsSynced != nil {
			go wait.UntilWithContext(ctx, c.runJobWorker, time.Second)
		}
		if c.daemonSetWatchEnabled {
			go wait.UntilWithContext(ctx, c.runDaemonSetWorker, time.Second)
		}
		if c.statefulSetWatchEnabled {
			go wait.UntilWithContext(ctx, c.runStatefulSetWorker, time.Second)
		}
		if c.pdbWatchEnabled {
			go wait.UntilWithContext(ctx, c.runPdbWorker, time.Second)
		}
		if c.cronJobWatchEnabled {
			go wait.UntilWithContext(ctx, c.runCronJobWorker, time.Second)
		}
		if c.hpaWatchEnabled {
			go wait.UntilWithContext(ctx, c.runHorizontalPodAutoscalerWorker, time.Second)
		}
		if c.serviceWatchEnabled {
			go wait.UntilWithContext(ctx, c.runServiceWorker, time.Second)
		}
		if c.endpointSliceWatchEnabled {
			go wait.UntilWithContext(ctx, c.runEndpointSliceWorker, time.Second)
		}
		if c.mwcWatchEnabled {
			go wait.UntilWithContext(ctx, c.runMwcWorker, time.Second)
		}
		if c.vwcWatchEnabled {
			go wait.UntilWithContext(ctx, c.runVwcWorker, time.Second)
		}
		if c.ingressWatchEnabled {
			go wait.UntilWithContext(ctx, c.runIngressWorker, time.Second)
		}
		if c.netpolWatchEnabled {
			go wait.UntilWithContext(ctx, c.runNetpolWorker, time.Second)
		}
		if c.cpWatchEnabled {
			go wait.UntilWithContext(ctx, c.runCpPodWorker, time.Second)
		}
	}

	<-ctx.Done()
	klog.InfoS("shutting down workers")
	return nil
}
