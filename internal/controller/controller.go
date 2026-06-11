package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/handler"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
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
	podsSynced             []cache.InformerSynced
	nodeLister             corev1lister.NodeLister
	nodesSynced            cache.InformerSynced
	deployLister           appsv1lister.DeploymentLister
	deploysSynced          []cache.InformerSynced
	jobLister              batchv1lister.JobLister
	jobsSynced             []cache.InformerSynced
	rsLister               appsv1lister.ReplicaSetLister
	rsSynced               []cache.InformerSynced
	dsLister               appsv1lister.DaemonSetLister
	dsSynced               []cache.InformerSynced
	ssLister               appsv1lister.StatefulSetLister
	ssSynced               []cache.InformerSynced
	deploymentWatchEnabled bool
	jobWatchEnabled        bool
}

func newFactories(client kubernetes.Interface, cfg *config.Config, resync time.Duration) (factorySet, []informers.SharedInformerFactory) {
	if len(cfg.AllowedNamespaces) <= 1 {
		var opts []informers.SharedInformerOption
		if len(cfg.AllowedNamespaces) == 1 {
			opts = append(opts, informers.WithNamespace(cfg.AllowedNamespaces[0]))
		}
		factory := informers.NewSharedInformerFactoryWithOptions(client, resync, opts...)
		return factorySet{global: factory}, []informers.SharedInformerFactory{factory}
	}

	factories := make([]informers.SharedInformerFactory, 0, len(cfg.AllowedNamespaces))
	for _, ns := range cfg.AllowedNamespaces {
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

func New(
	client kubernetes.Interface,
	cfg *config.Config,
	h handler.Handler,
) (*Controller, func()) {
	resync := time.Duration(cfg.ResyncSeconds) * time.Second

	fs, factories := newFactories(client, cfg, resync)

	podLister := fs.podLister()
	podInformers := fs.podInformers()

	var podsSynced []cache.InformerSynced
	for _, inf := range podInformers {
		podsSynced = append(podsSynced, inf.HasSynced)
	}

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
		podsSynced: podsSynced,
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
	syncFns := make([]cache.InformerSynced, 0, 1+len(c.podsSynced)+len(c.rsSynced)+len(c.dsSynced)+len(c.ssSynced)+len(c.deploysSynced)+len(c.jobsSynced))
	syncFns = append(syncFns, c.podsSynced...)
	syncFns = append(syncFns, c.rsSynced...)
	syncFns = append(syncFns, c.dsSynced...)
	syncFns = append(syncFns, c.ssSynced...)
	if c.nodesSynced != nil {
		syncFns = append(syncFns, c.nodesSynced)
	}
	syncFns = append(syncFns, c.deploysSynced...)
	syncFns = append(syncFns, c.jobsSynced...)
	if !cache.WaitForCacheSync(ctx.Done(), syncFns...) {
		return fmt.Errorf("failed to wait for caches to sync")
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

func (c *Controller) buildSeenSet() {
	pods, err := c.podLister.List(labels.Everything())
	if err != nil {
		klog.ErrorS(err, "failed to list pods for Seen set")
		return
	}
	var seen []string
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodSucceeded {
			continue
		}
		if pod.Status.Phase == corev1.PodPending || pod.Status.Phase == corev1.PodFailed {
			seen = append(seen, pod.Namespace+"/"+pod.Name)
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason != "ContainerCreating" && cs.State.Waiting.Reason != "PodInitializing" {
				seen = append(seen, pod.Namespace+"/"+pod.Name)
				break
			}
			if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 && cs.State.Terminated.Reason != "Completed" {
				seen = append(seen, pod.Namespace+"/"+pod.Name)
				break
			}
		}
	}
	if len(seen) > 0 {
		klog.V(4).InfoS("Seen set built", "count", len(seen))
		c.handler.SetSeen(seen)
	}
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
