package controller

import (
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/incident"
)

type Controller struct {
	components RuntimeSet
	client     kubernetes.Interface

	pipelineSet

	baselineRuntime
	// seedThresholds are the sustain windows the startup seeding applies, so
	// a seeded key matches the one the live path will build. Passing zero
	// here used the detectors' built-in defaults, which are not necessarily
	// the operator's configured ones -- and a key that differs from the live
	// key is a baseline entry that suppresses nothing.
	graphRuntime
	sourceSet

	readyFn          func()
	startupSummaryCh chan map[string]int
	scopeState
	diagnosticState
	now func() time.Time
}

// resolveNamespaces decides which namespaces to watch.
// If NamespaceSelector is set, it lists namespaces via k8s API using the label
// selector. Otherwise it uses the static AllowedNamespaces/ForbiddenNamespaces.

// NewWithRuntimeConfig constructs a controller from the immutable runtime
// snapshot. The YAML-facing Config remains at the application boundary.
func NewWithRuntimeConfig(
	client kubernetes.Interface,
	runtime config.RuntimeConfig,
	components RuntimeSet,
	dependencies RuntimeDependencies,
) (*Controller, func(), error) {
	if dependencies.Now == nil {
		return nil, nil, fmt.Errorf("controller clock is required")
	}
	resync := runtime.Lifecycle().ResyncInterval()

	scope, err := resolveNamespaces(runtime, client)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve namespaces: %w", err)
	}

	fs, factories := newFactories(
		client, scope, runtime.Scope().ForbiddenNamespaces(), resync,
	)

	podLister := fs.podLister()
	podInformers := fs.podInformers()

	maxBaseline := runtime.Persistence().MaxBaseline()
	if maxBaseline <= 0 {
		maxBaseline = incident.DefaultMaxBaseline
	}

	c := &Controller{
		components: components,
		client:     client,
		baselineRuntime: baselineRuntime{
			maxBaseline:    maxBaseline,
			seedThresholds: newSeedThresholds(runtime),
		},
		graphRuntime: graphRuntime{},
		pipelineSet: pipelineSet{
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
			resourceQuota: newResourcePipeline(
				"resourcequota", "resourcequotas",
			),
			limitRange: newResourcePipeline("limitrange", "limitranges"),
			namespace:  newResourcePipeline("namespace", "namespaces"),
			lease:      newResourcePipeline("lease", "leases"),
			replicaSet: newResourcePipeline("replicaset", "replicasets-status"),
		},
		podLister: podLister,
		scopeState: scopeState{
			watchAll: scope.all,
			forbiddenNamespaces: makeNamespaceSet(
				runtime.Scope().ForbiddenNamespaces(),
			),
		},
		now:              dependencies.Now,
		readyFn:          dependencies.Ready,
		startupSummaryCh: make(chan map[string]int, 1),
	}
	c.tracker = dependencies.Tracker
	c.graph = dependencies.Graph
	if !scope.all {
		c.allowedNamespaces = make(map[string]struct{}, len(scope.namespaces))
		for _, namespace := range scope.namespaces {
			c.allowedNamespaces[namespace] = struct{}{}
		}
	}
	for _, pipeline := range c.allPipelines() {
		pipeline.now = c.nowTime
		pipeline.queueDepth = c.queueDepth
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
		inf.AddEventHandler(safeEventHandler("pod", c.podEventHandler()))
	}

	c.wireNode(runtime, fs)
	c.wireRollout(runtime, fs)
	c.wireJobs(runtime, fs)
	c.wireDaemonSetMonitor(runtime, fs)
	c.wireCronJobs(runtime, fs)
	c.wireHPA(runtime, fs)
	c.wireService(runtime, fs)
	c.wireAdmissionWebhooks(runtime, fs)
	c.wireIngress(runtime, fs)
	c.wireNetpol(runtime, fs)
	c.wireClusterResources(runtime, fs)
	if runtime.Monitors().ControlPlane().Enabled {
		factories = append(factories, c.wireControlPlane(client, resync))
	}
	c.wireReplicaSet(runtime, fs)
	c.wireDaemonSetLister(fs)
	c.wireStatefulSet(runtime, fs)
	c.wirePDB(runtime, fs)
	factories = append(factories, c.wireEvents(client, resync, scope)...)
	if runtime.Monitors().ClusterAutoscaler().Enabled {
		factories = append(
			factories,
			wireClusterAutoscaler(c.components.Integration.Events, client, resync),
		)
	}
	c.wireConfigMap(fs)
	c.wireGraphSupport(fs)
	c.wireGraphHandlers(fs, runtime)
	if runtime.Monitors().TLS().Enabled {
		factories = append(factories, c.wireTLS(client, resync, scope)...)
	}

	if err := configureDirectRuntimes(c, components); err != nil {
		return nil, nil, fmt.Errorf("configure monitor sources: %w", err)
	}

	stopCh := make(chan struct{})
	for _, f := range factories {
		f.Start(stopCh)
	}

	if runtime.Monitors().NodeResource().Enabled {
		nc := runtime.Monitors().NodeResource()
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

func (c *Controller) queueDepth() int64 {
	var depth int64
	for _, pipeline := range c.allPipelines() {
		if pipeline != nil {
			depth += int64(pipeline.queue.Len())
		}
	}
	return depth
}

// StartupSummaries returns the one-time baseline summary stream. The
// controller publishes counts only; application lifecycle code decides
// whether and how to deliver a notification.
func (c *Controller) StartupSummaries() <-chan map[string]int {
	return c.startupSummaryCh
}
