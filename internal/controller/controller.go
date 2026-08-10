package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	admissionregistrationv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"
	storagev1lister "k8s.io/client-go/listers/storage/v1"
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

	tracker              *kwcontext.ChangeTracker
	graph                *kwcontext.ResourceGraph
	configMapLister      corev1lister.ConfigMapLister
	configMapSynced      []cache.InformerSynced
	pvcLister            corev1lister.PersistentVolumeClaimLister
	pvLister             corev1lister.PersistentVolumeLister
	serviceAccountLister corev1lister.ServiceAccountLister
	storageClassLister   storagev1lister.StorageClassLister

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

	c.wireNode(h, cfg, fs)
	c.wireRollout(h, cfg, fs)
	c.wireJobs(h, cfg, fs)
	c.wireDaemonSetMonitor(h, cfg, fs)
	c.wireCronJobs(h, cfg, fs)
	c.wireHPA(h, cfg, fs)
	c.wireService(h, cfg, fs)
	c.wireAdmissionWebhooks(h, cfg, fs)
	c.wireIngress(h, cfg, fs)
	c.wireNetpol(h, cfg, fs)
	if cfg.ControlPlaneMonitor.Enabled {
		factories = append(factories, c.wireControlPlane(h, client, resync))
	}
	c.wireReplicaSet(h, fs)
	c.wireDaemonSetLister(h, fs)
	c.wireStatefulSet(h, cfg, fs)
	c.wirePDB(h, cfg, fs)
	factories = append(factories, c.wireEvents(h, client, resync, namespaces)...)
	if cfg.ClusterAutoscalerMonitor.Enabled {
		factories = append(factories, wireClusterAutoscaler(h, client, resync))
	}
	c.wireConfigMap(fs)
	c.wireGraphSupport(fs)
	c.wireGraphHandlers(fs, cfg)
	if cfg.TlsMonitor.Enabled {
		factories = append(factories, c.wireTLS(h, client, resync, namespaces)...)
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
