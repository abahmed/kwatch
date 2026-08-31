package controller

import (
	"fmt"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	admregv1lister "k8s.io/client-go/listers/admissionregistration/v1"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	autoscalingv2lister "k8s.io/client-go/listers/autoscaling/v2"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	coordinationv1lister "k8s.io/client-go/listers/coordination/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	discoveryv1lister "k8s.io/client-go/listers/discovery/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
	policyv1lister "k8s.io/client-go/listers/policy/v1"
	storagev1lister "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/metrics"
)

type Controller struct {
	handler handler.Handler
	client  kubernetes.Interface

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
	resourceQuota *resourcePipeline
	limitRange    *resourcePipeline
	namespace     *resourcePipeline
	lease         *resourcePipeline
	replicaSet    *resourcePipeline

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
	// one per event informer, all indexed by "byPod"
	eventIndexers []cache.Indexer
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
	graphSynced   []cache.InformerSynced

	serviceLister       corev1lister.ServiceLister
	endpointSliceLister discoveryv1lister.EndpointSliceLister
	mwcLister           admregv1lister.MutatingWebhookConfigurationLister
	vwcLister           admregv1lister.ValidatingWebhookConfigurationLister
	ingressLister       networkingv1lister.IngressLister
	netpolLister        networkingv1lister.NetworkPolicyLister
	cpPodLister         corev1lister.PodLister
	resourceQuotaLister corev1lister.ResourceQuotaLister
	limitRangeLister    corev1lister.LimitRangeLister
	namespaceLister     corev1lister.NamespaceLister
	leaseLister         coordinationv1lister.LeaseLister

	nodeResourceCfg *config.NodeResourceMonitor

	readyFn                  func()
	watchAll                 bool
	allowedNamespaces        map[string]struct{}
	informerMu               sync.RWMutex
	informerEvents           int64
	informerWatchErrors      int64
	informerLastEvent        time.Time
	informerLastWatchError   time.Time
	informerLastWatchMessage string
	informers                []cache.SharedIndexInformer
	now                      func() time.Time
}

type InformerStatus struct {
	State                  string        `json:"state"`
	LastEvent              time.Time     `json:"lastEvent"`
	LastWatchError         time.Time     `json:"lastWatchError"`
	WatchErrors            int64         `json:"watchErrors"`
	Events                 int64         `json:"events"`
	EventAge               time.Duration `json:"eventAge"`
	WatchHealthy           bool          `json:"watchHealthy"`
	LastError              string        `json:"lastError,omitempty"`
	InformerCount          int           `json:"informerCount"`
	Unsynced               int           `json:"unsynced"`
	WithoutResourceVersion int           `json:"withoutResourceVersion"`
}

func (c *Controller) nowTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return clock.Now()
}

func (c *Controller) InformerStatus() interface{} {
	c.informerMu.RLock()
	defer c.informerMu.RUnlock()
	now := c.nowTime()
	status := InformerStatus{State: "unavailable", LastEvent: c.informerLastEvent, LastWatchError: c.informerLastWatchError, WatchErrors: c.informerWatchErrors, Events: c.informerEvents, LastError: c.informerLastWatchMessage, WatchHealthy: c.informerWatchErrors == 0 || now.Sub(c.informerLastWatchError) > 5*time.Minute, InformerCount: len(c.informers)}
	for _, informer := range c.informers {
		if !informer.HasSynced() {
			status.Unsynced++
		}
		if informer.LastSyncResourceVersion() == "" {
			status.WithoutResourceVersion++
		}
	}
	if !status.LastEvent.IsZero() {
		status.EventAge = now.Sub(status.LastEvent)
	}
	switch {
	case status.Unsynced > 0:
		status.State = "partial"
	case status.InformerCount > 0 && !status.WatchHealthy:
		status.State = "unavailable"
	case status.InformerCount > 0:
		status.State = "healthy"
	}
	return status
}

func (c *Controller) recordInformerEvent() {
	c.informerMu.Lock()
	c.informerEvents++
	metrics.Default.InformerEvents.Add(1)
	c.informerLastEvent = c.nowTime()
	c.informerMu.Unlock()
}

func (c *Controller) recordInformerWatchError(err error) {
	c.informerMu.Lock()
	c.informerWatchErrors++
	metrics.Default.InformerWatchErrors.Add(1)
	c.informerLastWatchError = c.nowTime()
	c.informerLastWatchMessage = err.Error()
	c.informerMu.Unlock()
}

// allPipelines returns every pipeline in a fixed order for iteration over
// shutdown, cache sync, and worker start.
func (c *Controller) allPipelines() []*resourcePipeline {
	return []*resourcePipeline{
		c.pod, c.node, c.deployment, c.job, c.daemonSet, c.statefulSet,
		c.pdb, c.cronJob, c.hpa, c.service, c.endpointSlice, c.mwc,
		c.vwc, c.ingress, c.netpol, c.cpPod, c.resourceQuota, c.limitRange, c.namespace, c.lease, c.replicaSet,
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
) (*Controller, func(), error) {
	resync := time.Duration(cfg.ResyncSeconds) * time.Second

	scope, err := resolveNamespaces(cfg, client)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve namespaces: %w", err)
	}

	fs, factories := newFactories(client, scope, cfg.ForbiddenNamespaces, resync)

	podLister := fs.podLister()
	podInformers := fs.podInformers()

	maxBaseline := cfg.Correlation.MaxBaseline
	if maxBaseline <= 0 {
		maxBaseline = correlation.DefaultMaxBaseline
	}

	c := &Controller{
		handler:       h,
		client:        client,
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
		mwc: newResourcePipeline(
			"mutatingwebhookconfiguration",
			"mutatingwebhookconfigurations",
		),
		vwc: newResourcePipeline(
			"validatingwebhookconfiguration",
			"validatingwebhookconfigurations",
		),
		ingress: newResourcePipeline("ingress", "ingresses"),
		netpol:  newResourcePipeline("networkpolicy", "networkpolicies"),
		cpPod: newResourcePipeline(
			"controlplane pod",
			"controlplanepods",
		),
		resourceQuota: newResourcePipeline("resourcequota", "resourcequotas"),
		limitRange:    newResourcePipeline("limitrange", "limitranges"),
		namespace:     newResourcePipeline("namespace", "namespaces"),
		lease:         newResourcePipeline("lease", "leases"),
		replicaSet:    newResourcePipeline("replicaset", "replicasets-status"),
		podLister:     podLister,
		maxBaseline:   maxBaseline,
		watchAll:      scope.all,
		now:           clock.Now,
	}
	if !scope.all {
		c.allowedNamespaces = make(map[string]struct{}, len(scope.namespaces))
		for _, namespace := range scope.namespaces {
			c.allowedNamespaces[namespace] = struct{}{}
		}
	}
	for _, pipeline := range c.allPipelines() {
		pipeline.now = c.nowTime
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
	c.resourceQuota.syncFn = c.syncResourceQuota
	c.limitRange.syncFn = c.syncLimitRange
	c.namespace.syncFn = c.syncNamespace
	c.lease.syncFn = c.syncLease
	c.replicaSet.syncFn = c.syncReplicaSet

	for _, inf := range podInformers {
		c.pod.synced = append(c.pod.synced, inf.HasSynced)
		inf.AddEventHandler(c.podEventHandler())
	}

	c.wireNode(cfg, fs)
	c.wireRollout(cfg, fs)
	c.wireJobs(cfg, fs)
	c.wireDaemonSetMonitor(cfg, fs)
	c.wireCronJobs(cfg, fs)
	c.wireHPA(cfg, fs)
	c.wireService(cfg, fs)
	c.wireAdmissionWebhooks(cfg, fs)
	c.wireIngress(cfg, fs)
	c.wireNetpol(cfg, fs)
	c.wireClusterResources(cfg, fs)
	if cfg.ControlPlaneMonitor.Enabled {
		factories = append(factories, c.wireControlPlane(client, resync))
	}
	c.wireReplicaSet(cfg, fs)
	c.wireDaemonSetLister(fs)
	c.wireStatefulSet(cfg, fs)
	c.wirePDB(cfg, fs)
	factories = append(factories, c.wireEvents(client, resync, scope)...)
	if cfg.ClusterAutoscalerMonitor.Enabled {
		factories = append(factories, wireClusterAutoscaler(h, client, resync))
	}
	c.wireConfigMap(fs)
	c.wireGraphSupport(fs)
	c.wireGraphHandlers(fs, cfg)
	if cfg.TlsMonitor.Enabled {
		factories = append(factories, c.wireTLS(client, resync, scope)...)
	}

	// Every informer is wired; hand the handler its lookups in one go. A
	// lister for a disabled monitor is nil, which the handler treats as
	// "not available" the same way the old per-lister setters did by never
	// being called.
	h.SetListers(handler.Listers{
		Pod:            c.podLister,
		Node:           c.nodeLister,
		Deploy:         c.deployLister,
		Job:            c.jobLister,
		CronJob:        c.cronJobLister,
		RS:             c.rsLister,
		DS:             c.dsLister,
		SS:             c.ssLister,
		PDB:            c.pdbLister,
		Event:          c.eventLister,
		EventsByPod:    c.eventsByPod,
		HPA:            c.hpaLister,
		MWC:            c.mwcLister,
		VWC:            c.vwcLister,
		Service:        c.serviceLister,
		EndpointSlice:  c.endpointSliceLister,
		ConfigMap:      c.configMapLister,
		Secret:         c.secretLister,
		ServiceAccount: c.serviceAccountLister,
		Netpol:         c.netpolLister,
		Ingress:        c.ingressLister,
		ResourceQuota:  c.resourceQuotaLister,
		LimitRange:     c.limitRangeLister,
		Namespace:      c.namespaceLister,
		Lease:          c.leaseLister,
		CPPod:          c.cpPodLister,
	})
	h.SetNamespaceScope(scope.namespaces, scope.all)

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

	return c, cleanup, nil
}
