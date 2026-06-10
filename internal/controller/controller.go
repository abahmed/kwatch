package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/handler"
	"k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
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
	podLister              corev1lister.PodLister
	podsSynced             cache.InformerSynced
	nodeLister             corev1lister.NodeLister
	nodesSynced            cache.InformerSynced
	deployLister           appsv1lister.DeploymentLister
	deploysSynced          cache.InformerSynced
	jobLister              batchv1lister.JobLister
	jobsSynced             cache.InformerSynced
	deploymentWatchEnabled bool
	jobWatchEnabled        bool
}

func New(
	client kubernetes.Interface,
	cfg *config.Config,
	h handler.Handler,
) (*Controller, func()) {
	var opts []informers.SharedInformerOption
	if len(cfg.AllowedNamespaces) == 1 {
		opts = append(opts, informers.WithNamespace(cfg.AllowedNamespaces[0]))
	}

	resync := time.Duration(cfg.ResyncSeconds) * time.Second

	factory := informers.NewSharedInformerFactoryWithOptions(
		client,
		resync,
		opts...,
	)

	podInformer := factory.Core().V1().Pods().Informer()
	podLister := factory.Core().V1().Pods().Lister()

	c := &Controller{
		handler: h,
		podQueue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[string](),
			workqueue.TypedRateLimitingQueueConfig[string]{Name: "pods"},
		),
		nodeQueue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[string](),
			workqueue.TypedRateLimitingQueueConfig[string]{Name: "nodes"},
		),
		deploymentQueue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[string](),
			workqueue.TypedRateLimitingQueueConfig[string]{Name: "deployments"},
		),
		jobQueue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[string](),
			workqueue.TypedRateLimitingQueueConfig[string]{Name: "jobs"},
		),
		podLister:  podLister,
		podsSynced: podInformer.HasSynced,
	}

	h.SetPodLister(podLister)

	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.enqueuePod,
		UpdateFunc: func(old, new interface{}) { c.enqueuePod(new) },
		DeleteFunc: c.enqueuePod,
	})

	if cfg.NodeMonitor.Enabled {
		nodeInformer := factory.Core().V1().Nodes().Informer()
		nodeLister := factory.Core().V1().Nodes().Lister()

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
		deployInformer := factory.Apps().V1().Deployments().Informer()
		deployLister := factory.Apps().V1().Deployments().Lister()

		c.deployLister = deployLister
		c.deploysSynced = deployInformer.HasSynced
		c.deploymentWatchEnabled = true

		h.SetDeploymentLister(deployLister)

		deployInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    c.enqueueDeployment,
			UpdateFunc: func(old, new interface{}) { c.enqueueDeployment(new) },
			DeleteFunc: c.enqueueDeployment,
		})
	}

	if cfg.JobMonitor.Enabled {
		jobInformer := factory.Batch().V1().Jobs().Informer()
		jobLister := factory.Batch().V1().Jobs().Lister()

		c.jobLister = jobLister
		c.jobsSynced = jobInformer.HasSynced
		c.jobWatchEnabled = true

		h.SetJobLister(jobLister)

		jobInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    c.enqueueJob,
			UpdateFunc: func(old, new interface{}) { c.enqueueJob(new) },
			DeleteFunc: c.enqueueJob,
		})
	}

	stopCh := make(chan struct{})
	factory.Start(stopCh)

	cleanup := func() {
		close(stopCh)
		factory.Shutdown()
	}

	return c, cleanup
}

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

func (c *Controller) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.podQueue.ShutDown()
	defer c.nodeQueue.ShutDown()
	defer c.deploymentQueue.ShutDown()
	defer c.jobQueue.ShutDown()

	klog.InfoS("starting controller")

	klog.InfoS("waiting for informer caches to sync")
	syncFns := []cache.InformerSynced{c.podsSynced}
	if c.nodesSynced != nil {
		syncFns = append(syncFns, c.nodesSynced)
	}
	if c.deploysSynced != nil {
		syncFns = append(syncFns, c.deploysSynced)
	}
	if c.jobsSynced != nil {
		syncFns = append(syncFns, c.jobsSynced)
	}
	if !cache.WaitForCacheSync(ctx.Done(), syncFns...) {
		return fmt.Errorf("failed to wait for caches to sync")
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
	}

	<-ctx.Done()
	klog.InfoS("shutting down workers")
	return nil
}

func (c *Controller) runPodWorker(ctx context.Context) {
	for c.processNextPodItem() {
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

func (c *Controller) processNextPodItem() bool {
	key, quit := c.podQueue.Get()
	if quit {
		return false
	}
	defer c.podQueue.Done(key)

	if err := c.syncPod(key); err != nil {
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

func (c *Controller) syncPod(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	pod, err := c.podLister.Pods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handler.ProcessPod(key, true)
		}
		return err
	}

	return c.handler.ProcessPodObject(pod, false)
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
