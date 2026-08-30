package controller

import (
	"fmt"
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

	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/handler"
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
	resourceQuota *resourcePipeline
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
	namespaceLister     corev1lister.NamespaceLister
	leaseLister         coordinationv1lister.LeaseLister

	nodeResourceCfg *config.NodeResourceMonitor

	readyFn           func()
	watchAll          bool
	allowedNamespaces map[string]struct{}
}

// allPipelines returns every pipeline in a fixed order for iteration over
// shutdown, cache sync, and worker start.
func (c *Controller) allPipelines() []*resourcePipeline {
	return []*resourcePipeline{
		c.pod, c.node, c.deployment, c.job, c.daemonSet, c.statefulSet,
		c.pdb, c.cronJob, c.hpa, c.service, c.endpointSlice, c.mwc,
		c.vwc, c.ingress, c.netpol, c.cpPod, c.resourceQuota, c.namespace, c.lease, c.replicaSet,
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
		namespace:     newResourcePipeline("namespace", "namespaces"),
		lease:         newResourcePipeline("lease", "leases"),
		replicaSet:    newResourcePipeline("replicaset", "replicasets-status"),
		podLister:     podLister,
		maxBaseline:   maxBaseline,
		watchAll:      scope.all,
	}
	if !scope.all {
		c.allowedNamespaces = make(map[string]struct{}, len(scope.namespaces))
		for _, namespace := range scope.namespaces {
			c.allowedNamespaces[namespace] = struct{}{}
		}
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
		Pod:           c.podLister,
		Node:          c.nodeLister,
		Deploy:        c.deployLister,
		Job:           c.jobLister,
		CronJob:       c.cronJobLister,
		RS:            c.rsLister,
		DS:            c.dsLister,
		SS:            c.ssLister,
		PDB:           c.pdbLister,
		Event:         c.eventLister,
		EventsByPod:   c.eventsByPod,
		HPA:           c.hpaLister,
		MWC:           c.mwcLister,
		VWC:           c.vwcLister,
		Service:       c.serviceLister,
		EndpointSlice: c.endpointSliceLister,
		Secret:        c.secretLister,
		Netpol:        c.netpolLister,
		Ingress:       c.ingressLister,
		ResourceQuota: c.resourceQuotaLister,
		Namespace:     c.namespaceLister,
		Lease:         c.leaseLister,
		CPPod:         c.cpPodLister,
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
