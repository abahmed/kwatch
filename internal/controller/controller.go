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
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/resource"
)

type Controller struct {
	handler handler.Handler

	// One pipeline per watched kind with its own queue and workers.
	pod           *resourcePipeline
	node          *resourcePipeline
	deployment    *resourcePipeline
	job           *resourcePipeline
	daemonSet     *resourcePipeline
	statefulSet   *resourcePipeline
	pdb           *resourcePipeline
	cronJob       *resourcePipeline
	hpa           *resourcePipeline
	service       *resourcePipeline
	endpointSlice *resourcePipeline
	mwc           *resourcePipeline
	vwc           *resourcePipeline
	ingress       *resourcePipeline
	netpol        *resourcePipeline
	cpPod         *resourcePipeline

	podLister     corev1lister.PodLister
	nodeLister    corev1lister.NodeLister
	deployLister  appsv1lister.DeploymentLister
	jobLister     batchv1lister.JobLister
	cronJobLister batchv1lister.CronJobLister
	rsLister      appsv1lister.ReplicaSetLister
	dsLister      appsv1lister.DaemonSetLister
	ssLister      appsv1lister.StatefulSetLister
	pdbLister     policyv1lister.PodDisruptionBudgetLister
	eventLister   corev1lister.EventLister
	hpaLister     autoscalingv2lister.HorizontalPodAutoscalerLister
	secretLister  corev1lister.SecretLister
	maxBaseline   int

	tracker              *kwcontext.ChangeTracker
	graph                *kwcontext.ResourceGraph
	configMapLister      corev1lister.ConfigMapLister
	configMapSynced      []cache.InformerSynced
	pvcLister            corev1lister.PersistentVolumeClaimLister
	pvLister             corev1lister.PersistentVolumeLister
	serviceAccountLister corev1lister.ServiceAccountLister
	storageClassLister   storagev1lister.StorageClassLister

	rsSynced      []cache.InformerSynced
	dsSynced      []cache.InformerSynced
	ssSynced      []cache.InformerSynced
	eventsSynced  []cache.InformerSynced
	secretsSynced []cache.InformerSynced

	serviceLister       corev1lister.ServiceLister
	endpointSliceLister discoveryv1lister.EndpointSliceLister
	mwcLister           admissionregistrationv1lister.MutatingWebhookConfigurationLister
	vwcLister           admissionregistrationv1lister.ValidatingWebhookConfigurationLister
	ingressLister       networkingv1lister.IngressLister
	netpolLister        networkingv1lister.NetworkPolicyLister
	cpPodLister         corev1lister.PodLister

	nodeResourceCfg *config.NodeResourceMonitor

	readyFn func()
}

// allPipelines returns every pipeline in a fixed order for iteration over
// shutdown, cache sync, and worker start.
func (c *Controller) allPipelines() []*resourcePipeline {
	return []*resourcePipeline{
		c.pod, c.node, c.deployment, c.job, c.daemonSet, c.statefulSet,
		c.pdb, c.cronJob, c.hpa, c.service, c.endpointSlice, c.mwc,
		c.vwc, c.ingress, c.netpol, c.cpPod,
	}
}

// activePipelines returns the pipelines whose watches were wired during New.
func (c *Controller) activePipelines() []*resourcePipeline {
	var active []*resourcePipeline
	for _, p := range c.allPipelines() {
		if p.startWorkers {
			active = append(active, p)
		}
	}
	return active
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

	maxBaseline := cfg.Correlation.MaxBaseline
	if maxBaseline <= 0 {
		maxBaseline = correlation.DefaultMaxBaseline
	}

	c := &Controller{
		handler:       h,
		pod:           newResourcePipeline("pod", "pods"),
		node:          newResourcePipeline("node", "nodes"),
		deployment:    newResourcePipeline("deployment", "deployments"),
		job:           newResourcePipeline("job", "jobs"),
		daemonSet:     newResourcePipeline("daemonset", "daemonsets"),
		statefulSet:   newResourcePipeline("statefulset", "statefulsets"),
		pdb:           newResourcePipeline("pdb", "poddisruptionbudgets"),
		cronJob:       newResourcePipeline("cronjob", "cronjobs"),
		hpa:           newResourcePipeline("hpa", "horizontalpodautoscalers"),
		service:       newResourcePipeline("service", "services"),
		endpointSlice: newResourcePipeline("endpointslice", "endpointslices"),
		mwc:           newResourcePipeline("mutatingwebhookconfiguration", "mutatingwebhookconfigurations"),
		vwc:           newResourcePipeline("validatingwebhookconfiguration", "validatingwebhookconfigurations"),
		ingress:       newResourcePipeline("ingress", "ingresses"),
		netpol:        newResourcePipeline("networkpolicy", "networkpolicies"),
		cpPod:         newResourcePipeline("controlplane pod", "controlplanepods"),
		podLister:     podLister,
		maxBaseline:   maxBaseline,
	}

	c.pod.startWorkers = true
	c.hpa.track = "horizontalpodautoscaler"
	c.cpPod.track = "pod"

	c.pod.syncFn = c.syncPod
	c.node.syncFn = c.syncNode
	c.deployment.syncFn = c.syncDeployment
	c.job.syncFn = c.syncJob
	c.daemonSet.syncFn = c.syncDaemonSet
	c.statefulSet.syncFn = c.syncStatefulSet
	c.pdb.syncFn = c.syncPdb
	c.cronJob.syncFn = c.syncCronJob
	c.hpa.syncFn = c.syncHorizontalPodAutoscaler
	c.service.syncFn = c.syncService
	c.endpointSlice.syncFn = c.syncEndpointSlice
	c.mwc.syncFn = c.syncMwc
	c.vwc.syncFn = c.syncVwc
	c.ingress.syncFn = c.syncIngress
	c.netpol.syncFn = c.syncNetpol
	c.cpPod.syncFn = c.syncCpPod

	h.SetPodLister(podLister)

	for _, inf := range podInformers {
		c.pod.synced = append(c.pod.synced, inf.HasSynced)
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
	for _, p := range c.allPipelines() {
		defer p.shutdown()
	}

	klog.InfoS("starting controller")

	klog.InfoS("waiting for informer caches to sync")
	var syncFns []cache.InformerSynced
	for _, p := range c.allPipelines() {
		syncFns = append(syncFns, p.synced...)
	}
	syncFns = append(syncFns, c.rsSynced...)
	syncFns = append(syncFns, c.dsSynced...)
	syncFns = append(syncFns, c.ssSynced...)
	syncFns = append(syncFns, c.eventsSynced...)
	syncFns = append(syncFns, c.configMapSynced...)
	syncFns = append(syncFns, c.secretsSynced...)
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
	if c.cpPod.startWorkers {
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
		for _, p := range c.activePipelines() {
			go wait.UntilWithContext(ctx, p.worker, time.Second)
		}
	}

	<-ctx.Done()
	klog.InfoS("shutting down workers")
	return nil
}
