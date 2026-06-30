package controller

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

type Controller struct {
	handler                handler.Handler
	podQueue               workqueue.TypedRateLimitingInterface[string]
	nodeQueue              workqueue.TypedRateLimitingInterface[string]
	deploymentQueue        workqueue.TypedRateLimitingInterface[string]
	jobQueue               workqueue.TypedRateLimitingInterface[string]
	daemonSetQueue         workqueue.TypedRateLimitingInterface[string]
	cronJobQueue           workqueue.TypedRateLimitingInterface[string]
	podLister              corev1lister.PodLister
	podsSynced             []cache.InformerSynced
	nodeLister             corev1lister.NodeLister
	nodesSynced            cache.InformerSynced
	deployLister           appsv1lister.DeploymentLister
	deploysSynced          []cache.InformerSynced
	jobLister              batchv1lister.JobLister
	jobsSynced             []cache.InformerSynced
	cronJobLister          batchv1lister.CronJobLister
	cronJobsSynced         []cache.InformerSynced
	rsLister               appsv1lister.ReplicaSetLister
	rsSynced               []cache.InformerSynced
	dsLister               appsv1lister.DaemonSetLister
	dsSynced               []cache.InformerSynced
	ssLister               appsv1lister.StatefulSetLister
	ssSynced               []cache.InformerSynced
	eventLister            corev1lister.EventLister
	eventsSynced           []cache.InformerSynced
	deploymentWatchEnabled bool
	jobWatchEnabled        bool
	daemonSetWatchEnabled  bool
	cronJobWatchEnabled    bool
	hpaQueue               workqueue.TypedRateLimitingInterface[string]
	hpaLister              autoscalingv2lister.HorizontalPodAutoscalerLister
	hpaSynced              []cache.InformerSynced
	hpaWatchEnabled        bool
	secretLister           corev1lister.SecretLister
	secretsSynced          []cache.InformerSynced
	maxBaseline            int

	readyFn func()
}

// resolveNamespaces decides which namespaces to watch.
// If NamespaceSelector is set, it lists namespaces via k8s API using the label
// selector. Otherwise it uses the static AllowedNamespaces/ForbiddenNamespaces.
func resolveNamespaces(cfg *config.Config, clientset kubernetes.Interface) ([]string, error) {
	if cfg.NamespaceSelector != "" {
		list, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{
			LabelSelector: cfg.NamespaceSelector,
		})
		if err != nil {
			return nil, fmt.Errorf("namespaceSelector list failed: %w", err)
		}
		ns := make([]string, 0, len(list.Items))
		for _, n := range list.Items {
			ns = append(ns, n.Name)
		}
		return ns, nil
	}
	return cfg.AllowedNamespaces, nil
}

func newFactories(client kubernetes.Interface, namespaces []string, resync time.Duration) (factorySet, []informers.SharedInformerFactory) {
	if len(namespaces) <= 1 {
		var opts []informers.SharedInformerOption
		if len(namespaces) == 1 {
			opts = append(opts, informers.WithNamespace(namespaces[0]))
		}
		factory := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
		return factorySet{global: factory}, []informers.SharedInformerFactory{factory}
	}

	factories := make([]informers.SharedInformerFactory, 0, len(namespaces))
	for _, ns := range namespaces {
		opts := []informers.SharedInformerOption{informers.WithNamespace(ns)}
		factories = append(factories, informers.NewSharedInformerFactoryWithOptions(client, resync, opts...))
	}
	return factorySet{perNamespace: factories}, factories
}

type factorySet struct {
	global       informers.SharedInformerFactory
	perNamespace []informers.SharedInformerFactory
}

func (fs factorySet) hasMultiple() bool { return len(fs.perNamespace) > 0 }

func (fs factorySet) podLister() corev1lister.PodLister {
	if fs.global != nil {
		return fs.global.Core().V1().Pods().Lister()
	}
	listers := make([]corev1lister.PodLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Core().V1().Pods().Lister())
	}
	return &multiPodLister{listers: listers}
}

func (fs factorySet) podInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Core().V1().Pods().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Core().V1().Pods().Informer())
	}
	return out
}

func (fs factorySet) nodeLister() corev1lister.NodeLister {
	if fs.global != nil {
		return fs.global.Core().V1().Nodes().Lister()
	}
	return fs.perNamespace[0].Core().V1().Nodes().Lister()
}

func (fs factorySet) nodeInformer() cache.SharedIndexInformer {
	if fs.global != nil {
		return fs.global.Core().V1().Nodes().Informer()
	}
	return fs.perNamespace[0].Core().V1().Nodes().Informer()
}

func (fs factorySet) deployLister() appsv1lister.DeploymentLister {
	if fs.global != nil {
		return fs.global.Apps().V1().Deployments().Lister()
	}
	listers := make([]appsv1lister.DeploymentLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Apps().V1().Deployments().Lister())
	}
	return &multiDeploymentLister{listers: listers}
}

func (fs factorySet) deployInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Apps().V1().Deployments().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Apps().V1().Deployments().Informer())
	}
	return out
}

func (fs factorySet) jobLister() batchv1lister.JobLister {
	if fs.global != nil {
		return fs.global.Batch().V1().Jobs().Lister()
	}
	listers := make([]batchv1lister.JobLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Batch().V1().Jobs().Lister())
	}
	return &multiJobLister{listers: listers}
}

func (fs factorySet) rsLister() appsv1lister.ReplicaSetLister {
	if fs.global != nil {
		return fs.global.Apps().V1().ReplicaSets().Lister()
	}
	listers := make([]appsv1lister.ReplicaSetLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Apps().V1().ReplicaSets().Lister())
	}
	return &multiReplicaSetLister{listers: listers}
}

func (fs factorySet) rsInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Apps().V1().ReplicaSets().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Apps().V1().ReplicaSets().Informer())
	}
	return out
}

func (fs factorySet) dsLister() appsv1lister.DaemonSetLister {
	if fs.global != nil {
		return fs.global.Apps().V1().DaemonSets().Lister()
	}
	listers := make([]appsv1lister.DaemonSetLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Apps().V1().DaemonSets().Lister())
	}
	return &multiDaemonSetLister{listers: listers}
}

func (fs factorySet) dsInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Apps().V1().DaemonSets().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Apps().V1().DaemonSets().Informer())
	}
	return out
}

func (fs factorySet) ssLister() appsv1lister.StatefulSetLister {
	if fs.global != nil {
		return fs.global.Apps().V1().StatefulSets().Lister()
	}
	listers := make([]appsv1lister.StatefulSetLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Apps().V1().StatefulSets().Lister())
	}
	return &multiStatefulSetLister{listers: listers}
}

func (fs factorySet) ssInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Apps().V1().StatefulSets().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Apps().V1().StatefulSets().Informer())
	}
	return out
}

func (fs factorySet) jobInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Batch().V1().Jobs().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Batch().V1().Jobs().Informer())
	}
	return out
}

func (fs factorySet) cronJobLister() batchv1lister.CronJobLister {
	if fs.global != nil {
		return fs.global.Batch().V1().CronJobs().Lister()
	}
	listers := make([]batchv1lister.CronJobLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Batch().V1().CronJobs().Lister())
	}
	return &multiCronJobLister{listers: listers}
}

func (fs factorySet) cronJobInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Batch().V1().CronJobs().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Batch().V1().CronJobs().Informer())
	}
	return out
}

func (fs factorySet) hpaLister() autoscalingv2lister.HorizontalPodAutoscalerLister {
	if fs.global != nil {
		return fs.global.Autoscaling().V2().HorizontalPodAutoscalers().Lister()
	}
	listers := make([]autoscalingv2lister.HorizontalPodAutoscalerLister, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		listers = append(listers, f.Autoscaling().V2().HorizontalPodAutoscalers().Lister())
	}
	return &multiHorizontalPodAutoscalerLister{listers: listers}
}

func (fs factorySet) hpaInformers() []cache.SharedIndexInformer {
	if fs.global != nil {
		return []cache.SharedIndexInformer{fs.global.Autoscaling().V2().HorizontalPodAutoscalers().Informer()}
	}
	out := make([]cache.SharedIndexInformer, 0, len(fs.perNamespace))
	for _, f := range fs.perNamespace {
		out = append(out, f.Autoscaling().V2().HorizontalPodAutoscalers().Informer())
	}
	return out
}

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
		handler:         h,
		podQueue:        workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "pods"}),
		nodeQueue:       workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "nodes"}),
		deploymentQueue: workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "deployments"}),
		jobQueue:        workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "jobs"}),
		daemonSetQueue:  workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "daemonsets"}),
		cronJobQueue:    workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "cronjobs"}),
		hpaQueue:        workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "horizontalpodautoscalers"}),
		podLister:       podLister,
		podsSynced:      podsSynced,
		maxBaseline:     maxBaseline,
	}

	h.SetPodLister(podLister)

	for _, inf := range podInformers {
		inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    c.enqueuePod,
			UpdateFunc: func(old, new interface{}) { c.enqueuePod(new) },
			DeleteFunc: c.enqueuePod,
		})
	}

	if cfg.NodeMonitor.Enabled {
		nodeInformer := fs.nodeInformer()
		nodeLister := fs.nodeLister()

		c.nodeLister = nodeLister
		c.nodesSynced = nodeInformer.HasSynced

		h.SetNodeLister(nodeLister)

		nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    c.enqueueNode,
			UpdateFunc: func(old, new interface{}) { c.enqueueNode(new) },
			DeleteFunc: c.enqueueNode,
		})
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
			inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
				AddFunc:    c.enqueueDeployment,
				UpdateFunc: func(old, new interface{}) { c.enqueueDeployment(new) },
				DeleteFunc: c.enqueueDeployment,
			})
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
			inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
				AddFunc:    c.enqueueJob,
				UpdateFunc: func(old, new interface{}) { c.enqueueJob(new) },
				DeleteFunc: c.enqueueJob,
			})
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
			inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
				AddFunc:    c.enqueueDaemonSet,
				UpdateFunc: func(old, new interface{}) { c.enqueueDaemonSet(new) },
				DeleteFunc: c.enqueueDaemonSet,
			})
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
			inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
				AddFunc:    c.enqueueCronJob,
				UpdateFunc: func(old, new interface{}) { c.enqueueCronJob(new) },
				DeleteFunc: c.enqueueCronJob,
			})
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
			inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
				AddFunc:    c.enqueueHorizontalPodAutoscaler,
				UpdateFunc: func(old, new interface{}) { c.enqueueHorizontalPodAutoscaler(new) },
				DeleteFunc: c.enqueueHorizontalPodAutoscaler,
			})
		}
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
			eventInformer.AddIndexers(cache.Indexers{
				"byPod": func(obj interface{}) ([]string, error) {
					ev, ok := obj.(*corev1.Event)
					if !ok {
						return nil, nil
					}
					return []string{ev.InvolvedObject.Name}, nil
				},
			})
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
				eventInformer.AddIndexers(cache.Indexers{
					"byPod": func(obj interface{}) ([]string, error) {
						ev, ok := obj.(*corev1.Event)
						if !ok {
							return nil, nil
						}
						return []string{ev.InvolvedObject.Name}, nil
					},
				})
				listers = append(listers, ef.Core().V1().Events().Lister())
				c.eventsSynced = append(c.eventsSynced, eventInformer.HasSynced)
			}
			c.eventLister = &multiEventLister{listers: listers}
		}
		h.SetEventLister(c.eventLister)
		factories = append(factories, eventFactories...)
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

	cleanup := func() {
		close(stopCh)
		for _, f := range factories {
			f.Shutdown()
		}
	}

	return c, cleanup
}

func (c *Controller) SetReadyFunc(fn func()) { c.readyFn = fn }

func (c *Controller) enqueuePod(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.podQueue.Add(key)
}

func (c *Controller) enqueueNode(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.nodeQueue.Add(key)
}

func (c *Controller) enqueueDeployment(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.deploymentQueue.Add(key)
}

func (c *Controller) enqueueJob(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.jobQueue.Add(key)
}

func (c *Controller) enqueueDaemonSet(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.daemonSetQueue.Add(key)
}

func (c *Controller) enqueueCronJob(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.cronJobQueue.Add(key)
}

func (c *Controller) enqueueHorizontalPodAutoscaler(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.hpaQueue.Add(key)
}

func (c *Controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.podQueue.ShutDown()
	defer c.nodeQueue.ShutDown()
	defer c.deploymentQueue.ShutDown()
	defer c.jobQueue.ShutDown()
	defer c.daemonSetQueue.ShutDown()
	defer c.cronJobQueue.ShutDown()
	defer c.hpaQueue.ShutDown()

	klog.InfoS("starting controller")

	klog.InfoS("waiting for informer caches to sync")
	syncFns := make([]cache.InformerSynced, 0, 1+len(c.podsSynced)+len(c.rsSynced)+len(c.dsSynced)+len(c.ssSynced)+len(c.eventsSynced)+len(c.deploysSynced)+len(c.jobsSynced)+len(c.cronJobsSynced)+len(c.secretsSynced))
	syncFns = append(syncFns, c.podsSynced...)
	syncFns = append(syncFns, c.rsSynced...)
	syncFns = append(syncFns, c.dsSynced...)
	syncFns = append(syncFns, c.ssSynced...)
	syncFns = append(syncFns, c.eventsSynced...)
	if c.nodesSynced != nil {
		syncFns = append(syncFns, c.nodesSynced)
	}
	syncFns = append(syncFns, c.deploysSynced...)
	syncFns = append(syncFns, c.jobsSynced...)
	syncFns = append(syncFns, c.cronJobsSynced...)
	syncFns = append(syncFns, c.hpaSynced...)
	syncFns = append(syncFns, c.secretsSynced...)
	if !cache.WaitForCacheSync(ctx.Done(), syncFns...) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	if c.readyFn != nil {
		c.readyFn()
	}

	c.buildSeenSet()

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
		if c.cronJobWatchEnabled {
			go wait.UntilWithContext(ctx, c.runCronJobWorker, time.Second)
		}
		if c.hpaWatchEnabled {
			go wait.UntilWithContext(ctx, c.runHorizontalPodAutoscalerWorker, time.Second)
		}
	}

	<-ctx.Done()
	klog.InfoS("shutting down workers")
	return nil
}

func (c *Controller) runPodWorker(ctx context.Context) {
	for c.processNextPodItem(ctx) {
	}
}

func (c *Controller) runNodeWorker(ctx context.Context) {
	for c.processNextNodeItem() {
	}
}

func (c *Controller) runDeploymentWorker(ctx context.Context) {
	for c.processNextDeploymentItem() {
	}
}

func (c *Controller) runJobWorker(ctx context.Context) {
	for c.processNextJobItem() {
	}
}

func (c *Controller) runDaemonSetWorker(ctx context.Context) {
	for c.processNextDaemonSetItem() {
	}
}

func (c *Controller) runCronJobWorker(ctx context.Context) {
	for c.processNextCronJobItem() {
	}
}

func (c *Controller) runHorizontalPodAutoscalerWorker(ctx context.Context) {
	for c.processNextHorizontalPodAutoscalerItem() {
	}
}

func (c *Controller) processNextPodItem(ctx context.Context) bool {
	key, quit := c.podQueue.Get()
	if quit {
		return false
	}
	defer c.podQueue.Done(key)

	if err := c.syncPod(ctx, key); err != nil {
		c.podQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing pod %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.podQueue.Forget(key)
	return true
}

func (c *Controller) processNextNodeItem() bool {
	key, quit := c.nodeQueue.Get()
	if quit {
		return false
	}
	defer c.nodeQueue.Done(key)

	if err := c.syncNode(key); err != nil {
		c.nodeQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing node %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.nodeQueue.Forget(key)
	return true
}

func (c *Controller) processNextDeploymentItem() bool {
	key, quit := c.deploymentQueue.Get()
	if quit {
		return false
	}
	defer c.deploymentQueue.Done(key)

	if err := c.syncDeployment(key); err != nil {
		c.deploymentQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing deployment %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.deploymentQueue.Forget(key)
	return true
}

func (c *Controller) processNextJobItem() bool {
	key, quit := c.jobQueue.Get()
	if quit {
		return false
	}
	defer c.jobQueue.Done(key)

	if err := c.syncJob(key); err != nil {
		c.jobQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing job %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.jobQueue.Forget(key)
	return true
}

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
	add := func(key, pod string) {
		if total >= c.maxBaseline {
			return
		}
		if baseline[key] == nil {
			baseline[key] = map[string]int64{}
		}
		if _, exists := baseline[key][pod]; !exists {
			total++
		}
		baseline[key][pod] = now.Unix()

		// Derive owner/reason key for the suppressed count from the incident key.
		// Incident key format: namespace:owner:reason:container
		if i1 := strings.IndexByte(key, ':'); i1 >= 0 {
			if i2 := strings.LastIndexByte(key, ':'); i2 > i1 {
				ownerReason := key[i1+1 : i2]
				// Replace middle colon with slash: "owner:reason" → "owner/reason"
				if j := strings.IndexByte(ownerReason, ':'); j >= 0 {
					suppressed[ownerReason[:j]+"/"+ownerReason[j+1:]]++
				}
			}
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
				reason = w.Reason
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
	if c.nodeLister != nil {
		if nodes, err := c.nodeLister.List(labels.Everything()); err == nil {
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
	// Live owner-level signals set PodName="" (no PodName in process_*.go signals),
	// so we seed under the empty pod key for the baseline match.
	seedSignal := func(sig *event.Signal, name string) {
		ev := event.Event{
			Namespace: sig.Namespace,
			Reason:    sig.Reason,
		}
		key := correlation.IncidentKey(ev, sig.Owner, nil)
		add(key, "") // match the live isBaselined(key, "") lookup
	}

	// DaemonSets
	if c.dsLister != nil {
		if dss, err := c.dsLister.List(labels.Everything()); err == nil {
			for _, ds := range dss {
				if sig := handler.DetectDaemonSetIssue(ds); sig != nil {
					seedSignal(sig, ds.Name)
				}
			}
		}
	}

	// Deployments
	if c.deployLister != nil {
		if deploys, err := c.deployLister.List(labels.Everything()); err == nil {
			for _, deploy := range deploys {
				if sig := handler.DetectDeploymentIssue(deploy); sig != nil {
					seedSignal(sig, deploy.Name)
				}
			}
		}
	}

	// Jobs
	if c.jobLister != nil {
		if jobs, err := c.jobLister.List(labels.Everything()); err == nil {
			for _, job := range jobs {
				if sig := handler.DetectJobIssue(job); sig != nil {
					seedSignal(sig, job.Name)
				}
			}
		}
	}

	// CronJobs
	if c.cronJobLister != nil {
		if cjs, err := c.cronJobLister.List(labels.Everything()); err == nil {
			for _, cj := range cjs {
				if sig := handler.DetectCronJobIssue(cj); sig != nil {
					seedSignal(sig, cj.Name)
				}
			}
		}
	}

	// HPAs — seed both scaling errors and maxed-out conditions
	if c.hpaLister != nil {
		if hpas, err := c.hpaLister.List(labels.Everything()); err == nil {
			for _, hpa := range hpas {
				for _, sig := range handler.DetectHPAIssues(hpa) {
					seedSignal(sig, hpa.Name)
				}
			}
		}
	}

	if len(baseline) > 0 {
		klog.V(4).InfoS("Seen set built", "count", len(baseline))
		c.handler.SetSeen(baseline)
	}
	c.handler.ReportStartupSummary(suppressed)
}

func (c *Controller) syncPod(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	pod, err := c.podLister.Pods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessPod(ctx, key, true)
		}
		return err
	}

	return c.handler.ProcessPodObject(ctx, pod, false)
}

func (c *Controller) syncNode(key string) error {
	deleted := false
	_, err := c.nodeLister.Get(key)
	if err != nil {
		if errors.IsNotFound(err) {
			deleted = true
		} else {
			return err
		}
	}

	return c.handler.ProcessNode(key, deleted)
}

func (c *Controller) syncDeployment(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	deploy, err := c.deployLister.Deployments(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessDeployment(key, true)
		}
		return err
	}

	return c.handler.ProcessDeploymentObject(deploy, false)
}

func (c *Controller) processNextDaemonSetItem() bool {
	key, quit := c.daemonSetQueue.Get()
	if quit {
		return false
	}
	defer c.daemonSetQueue.Done(key)

	if err := c.syncDaemonSet(key); err != nil {
		c.daemonSetQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing daemonset %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.daemonSetQueue.Forget(key)
	return true
}

func (c *Controller) processNextCronJobItem() bool {
	key, quit := c.cronJobQueue.Get()
	if quit {
		return false
	}
	defer c.cronJobQueue.Done(key)

	if err := c.syncCronJob(key); err != nil {
		c.cronJobQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing cronjob %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.cronJobQueue.Forget(key)
	return true
}

func (c *Controller) syncDaemonSet(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	ds, err := c.dsLister.DaemonSets(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessDaemonSet(key, true)
		}
		return err
	}

	return c.handler.ProcessDaemonSetObject(ds, false)
}

func (c *Controller) syncCronJob(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	cj, err := c.cronJobLister.CronJobs(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessCronJob(key, true)
		}
		return err
	}

	return c.handler.ProcessCronJobObject(cj, false)
}

func (c *Controller) processNextHorizontalPodAutoscalerItem() bool {
	key, quit := c.hpaQueue.Get()
	if quit {
		return false
	}
	defer c.hpaQueue.Done(key)

	if err := c.syncHorizontalPodAutoscaler(key); err != nil {
		c.hpaQueue.AddRateLimited(key)
		utilruntime.HandleError(fmt.Errorf("error syncing hpa %q: %s, requeuing", key, err.Error()))
		return true
	}

	c.hpaQueue.Forget(key)
	return true
}

func (c *Controller) syncHorizontalPodAutoscaler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	hpa, err := c.hpaLister.HorizontalPodAutoscalers(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessHorizontalPodAutoscaler(key, true)
		}
		return err
	}

	return c.handler.ProcessHorizontalPodAutoscalerObject(hpa, false)
}

func (c *Controller) syncJob(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	job, err := c.jobLister.Jobs(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessJob(key, true)
		}
		return err
	}

	return c.handler.ProcessJobObject(job, false)
}
