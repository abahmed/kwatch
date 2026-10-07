package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Virtual kinds that have no Kubernetes object of their own.
const (
	kindClusterDNS inventory.Kind = "cluster-dns"
	kindAPIServer  inventory.Kind = "apiserver"
)

// anything matches every entity of every kind.
var anything = Side{Group: AnyGroup, Kind: AnyKind}

// configModes are the effect modes of a broken or missing reference.
var configModes = []detection.Mode{
	detection.ModeCreateError, ModeMissing, detection.ModeReference,
	detection.ModeIngressTLSSecretMissing, detection.ModeCrashLoop,
	detection.ModeCreating}

// podFailures are the modes of a pod that does not run properly.
var podFailures = []detection.Mode{
	detection.ModeCrashLoop, detection.ModeNotReady, detection.ModeImagePull,
	detection.ModeOOMKilled, detection.ModeCreateError, detection.ModeProbe,
	detection.ModeRestarting, detection.ModeCannotRun, detection.ModeExit,
	detection.ModeFailed, detection.ModeError}

// genericRows cover every kind. They have lower priors than the
// specific rows so a precise row wins when both match.
var genericRows = []Row{
	{
		// An owner that fails (not one that merely summarises its
		// pods) explains what it owns: an operator's custom resource
		// that fails explains its pods.
		Name: "owner-failing",
		Cause: Side{Group: AnyGroup, Kind: AnyKind, NotModes: summaryModes,
			Health: []detection.Health{detection.Failing}},
		Link: LinkOwns, Effect: anything, Prior: 0.6,
	},
	{
		// A missing or failing object explains the objects that name
		// it with a configuration or reference error.
		Name: "used-missing",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{ModeMissing}, Health: []detection.Health{
				detection.Failing}},
		Link: LinkUses,
		Effect: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: configModes},
		Prior: 0.6,
	},
	{
		// A failing controller explains the objects it writes that
		// stopped being reconciled.
		Name: "controller-failing",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Health: []detection.Health{detection.Failing}},
		Link: LinkManages,
		Effect: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{detection.ModeNotReconciling}},
		Prior: 0.7,
	},
	{
		// A Service without endpoints explains what it serves: an
		// admission webhook or an APIService. Inferred from the webhooks'
		// own findings (the Service itself may be scaled to zero), it
		// needs more than one webhook: a single webhook stays its own root.
		Name: "service-no-endpoints",
		Cause: Side{Kind: kube.KindService,
			Modes: []detection.Mode{
				detection.ModeNoEndpoints, detection.ModeBackendsDegraded}},
		Link: LinkServedBy, Effect: anything, Prior: 0.7,
		InferredMinWorkloads: SharedMinWorkloads,
	},
	{
		// An unavailable APIService explains every client of its API.
		Name: "apiservice-unavailable",
		Cause: Side{Kind: kube.KindAPIService,
			Modes: []detection.Mode{detection.ModeAPIServiceUnavailable,
				ModeMetricsUnserved}},
		Link: LinkServedBy, Effect: anything, Prior: 0.6,
	},
	{
		// A namespace being deleted explains what it contains.
		Name: "namespace-terminating",
		Cause: Side{Kind: kube.KindNamespace,
			Modes: []detection.Mode{
				detection.ModeStuckDeleting, detection.ModeTerminating}},
		Link: LinkContains, Effect: anything, Prior: 0.6,
	},
	{
		// Failing pods explain the Service they back.
		Name:  "backends-failing",
		Cause: Side{Kind: kube.KindPod},
		Link:  LinkBacks,
		Effect: Side{Kind: kube.KindService,
			Modes: []detection.Mode{
				detection.ModeNoEndpoints, detection.ModeBackendsDegraded}},
		Prior: 0.6,
	},
	{
		// An object whose own spec changed just before it failed broke
		// itself: a custom resource given an invalid value, a Service
		// given a selector that matches nothing, a route its gateway
		// no longer accepts. Only objects without an owner match.
		Name: "own-change",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{ModeSpecChanged}},
		Link:   LinkSelf,
		Effect: Side{Group: AnyGroup, Kind: AnyKind},
		Prior:  0.65,
	},
	{
		// An unavailable API server explains anything that stopped
		// being reconciled.
		Name: "apiserver-unavailable",
		Cause: Side{Kind: kindAPIServer,
			Modes: []detection.Mode{
				detection.ModeUnavailable, detection.ModeLatency}},
		Link: LinkServedBy,
		Effect: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{detection.ModeNotReconciling}},
		Prior: 0.5,
	},
}

// podSide matches pods (and their containers) in the given modes.
func podSide(modes ...detection.Mode) Side {
	return Side{Kind: kube.KindPod, Modes: modes}
}

// nodeRows cover the node conditions that hurt the pods on the node.
var nodeRows = []Row{
	{
		Name: "node-not-ready",
		Cause: Side{Kind: kube.KindNode, Modes: []detection.Mode{
			detection.ModeNotReady, detection.ModeHeartbeatStale,
			detection.ModeStuckDeleting}},
		Link: LinkRunsOn,
		Effect: podSide(
			detection.ModeNotReady, detection.ModeStatusUnknown,
			detection.ModeEvicted, detection.ModeUnreachable,
			detection.ModeFailed, detection.ModeStuckDeleting,
			detection.ModeProbe, detection.ModePending),
		Prior: 0.8,
	},
	{
		// Exit code 137 without OOMKilled is a plain SIGKILL, which
		// the kubelet sends when it reclaims memory.
		Name: "node-memory-pressure",
		Cause: Side{Kind: kube.KindNode,
			Modes: []detection.Mode{
				detection.ModeMemoryPressure, detection.ModeMemory}},
		Link: LinkRunsOn,
		Effect: podSide(
			detection.ModeEvicted, detection.ModeOOMKilled,
			detection.ModeFailed, detection.ModeError, detection.ModeExitKilled,
			detection.ModeKilled),
		Prior: 0.7,
	},
	{
		// Under memory pressure probes time out and pods turn
		// unready before anything is evicted. Weaker than an
		// eviction: unready pods have many other causes.
		Name: "node-memory-pressure-unready",
		Cause: Side{Kind: kube.KindNode,
			Modes: []detection.Mode{
				detection.ModeMemoryPressure, detection.ModeMemory}},
		Link:   LinkRunsOn,
		Effect: podSide(detection.ModeNotReady, detection.ModeProbe),
		Prior:  0.55,
	},
	{
		Name: "node-disk-pressure",
		Cause: Side{Kind: kube.KindNode, Modes: []detection.Mode{
			detection.ModeDiskPressure, detection.ModeDisk,
			detection.ModeFilesystem, detection.ModeInodes}},
		Link: LinkRunsOn,
		Effect: podSide(
			detection.ModeEvicted, detection.ModeFailed,
			detection.ModeCreateError, detection.ModeImagePull),
		Prior: 0.6,
	},
	{
		Name: "node-pid-pressure",
		Cause: Side{Kind: kube.KindNode, Modes: []detection.Mode{
			detection.ModePIDPressure}},
		Link: LinkRunsOn,
		Effect: podSide(
			detection.ModeEvicted, detection.ModeFailed,
			detection.ModeCannotRun, detection.ModeCrashLoop),
		Prior: 0.6,
	},
	{
		Name: "node-network",
		Cause: Side{Kind: kube.KindNode, Modes: []detection.Mode{
			detection.ModeNetworkUnavailable, detection.ModeNetwork,
			detection.ModeNetworkErrors}},
		Link: LinkRunsOn,
		Effect: podSide(
			detection.ModeNotReady, detection.ModeCreating,
			detection.ModeCreateError, detection.ModeProbe,
			detection.ModeCrashLoop),
		Prior: 0.7,
	},
	{
		// The node is short of memory and the limits of its pods add up
		// to more than it has: a pod killed within its own limit was
		// killed for the node's sake. The finding exists only while the
		// node is short, never for the limits alone. Weaker than memory
		// pressure, which the kubelet states outright.
		Name: "node-overcommitted",
		Cause: Side{Kind: kube.KindNode, Modes: []detection.Mode{
			detection.ModeMemoryOvercommitted}},
		Link: LinkRunsOn,
		Effect: podSide(
			detection.ModeOOMKilled, detection.ModeEvicted,
			detection.ModeExitKilled, detection.ModeKilled),
		Prior: 0.5,
	},
}

// workloadRows cover rollouts and references.
var workloadRows = []Row{
	{
		// A new template explains pods that fail right after it.
		Name: "rollout",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{ModeSpecChanged, ModeCreated}},
		Link: LinkOwns, Effect: podSide(podFailures...), Prior: 0.7,
	},
	{
		Name: "config-missing-or-changed",
		Cause: Side{Kind: kube.KindSecret, Modes: []detection.Mode{ModeMissing,
			ModeChanged}},
		// A Secret can be an image pull Secret: a pod that cannot
		// pull with a missing or rotated login fails with ImagePull.
		Link: LinkUses, Effect: podSide(
			append([]detection.Mode{detection.ModeImagePull},
				configModes...)...), Prior: 0.7,
	},
	{
		Name: "configmap-missing-or-changed",
		Cause: Side{Kind: kube.KindConfigMap, Modes: []detection.Mode{
			ModeMissing, ModeChanged}},
		Link: LinkUses, Effect: podSide(configModes...), Prior: 0.7,
	},
	{
		Name: "claim-not-usable",
		Cause: Side{Kind: kube.KindPVC, Modes: []detection.Mode{
			detection.ModeClaimFailed, detection.ModeVolume,
			detection.ModeVolumeFull,
			detection.ModeAttachFailed, detection.ModePending, ModeMissing}},
		Link: LinkMounts,
		Effect: podSide(
			detection.ModePending, detection.ModeUnschedulable,
			detection.ModeCreating, detection.ModeCrashLoop,
			detection.ModeCreateError, detection.ModeFailed),
		Prior: 0.7,
	},
	{
		Name: "policy-restricts",
		Cause: Side{Kind: kube.KindNetworkPolicy, Modes: []detection.Mode{
			ModeChanged}},
		Link: LinkRestricts,
		Effect: podSide(
			detection.ModeCrashLoop, detection.ModeNotReady,
			detection.ModeProbe),
		Prior: 0.6,
	},
}

// clusterRows cover shared services every workload depends on.
var clusterRows = []Row{
	{
		Name: "registry-refuses",
		Cause: Side{Kind: kube.KindRegistry, Modes: []detection.Mode{
			pullAuth, pullRateLimit, pullServer, pullTLS, pullNetwork,
			pullStatus, pullUnexplained}},
		Link: LinkPulls, Effect: podSide(detection.ModeImagePull), Prior: 0.7,
		MinCovered: 2,
	},
	{
		// The cluster DNS servers fail with the service they make
		// up: their crash is the cluster DNS outage, not a second
		// incident.
		Name: "cluster-dns-servers",
		Cause: Side{Kind: kindClusterDNS, Modes: []detection.Mode{
			detection.ModeUnavailable, detection.ModeActiveProbe,
			ModeResolution}},
		Link:   LinkContains,
		Effect: podSide(podFailures...),
		Prior:  0.7,
	},
	{
		Name: "cluster-dns-failing",
		Cause: Side{Kind: kindClusterDNS, Modes: []detection.Mode{
			detection.ModeUnavailable, detection.ModeActiveProbe,
			ModeResolution}},
		Link: LinkResolvesVia,
		Effect: Side{Kind: kube.KindPod, Modes: podFailures,
			Signal: SignalDNS},
		Prior: 0.7, InferredMinWorkloads: SharedMinWorkloads,
	},
	{
		Name: "webhook-rejects",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{
				detection.ModeWebhook, detection.ModeNoEndpoints}},
		Link: LinkAdmits,
		Effect: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{
				detection.ModeFailedCreate, detection.ModeReplicaFailure},
			Signal: SignalWebhook},
		Prior: 0.8,
	},
	{
		// Several webhooks calling one Service that does not exist fail
		// together; the missing Service is their one root. A single
		// webhook keeps its own root.
		Name: "webhook-backend-missing",
		Cause: Side{Kind: kube.KindService,
			Modes: []detection.Mode{ModeMissing}},
		Link: LinkServedBy,
		Effect: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{detection.ModeWebhookBackendMissing}},
		Prior: 0.8, InferredMinWorkloads: SharedMinWorkloads,
	},
	{
		Name: "metrics-api-down",
		Cause: Side{Kind: kube.KindAPIService,
			Modes: []detection.Mode{detection.ModeAPIServiceUnavailable,
				ModeMetricsUnserved}},
		Link: LinkServedBy,
		Effect: Side{Kind: kube.KindHPA,
			Modes: []detection.Mode{detection.ModeScalingNoMetrics}},
		Prior: 0.8, InferredMinWorkloads: SharedMinWorkloads,
	},
	{
		Name: "quota-exhausted",
		Cause: Side{Kind: kube.KindQuota,
			Modes: []detection.Mode{
				detection.ModeQuotaExhausted, detection.ModeQuota}},
		Link: LinkAdmits,
		Effect: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{
				detection.ModeFailedCreate, detection.ModeReplicaFailure},
			Signal: SignalQuota},
		Prior: 0.8,
	},
	{
		Name: "scheduler-capacity",
		Cause: Side{Kind: KindScheduling,
			Modes: []detection.Mode{ModeRejectsNodes}},
		Link:   LinkSchedules,
		Effect: podSide(detection.ModePending, detection.ModeUnschedulable),
		Prior:  0.7,
	},
	{
		// The scheduler names a volume node affinity conflict: the
		// pod's claim is bound to a volume in a zone with no node for
		// it. The claim is the cause, not the scheduler.
		Name: "claim-pins-pod",
		Cause: Side{Kind: kube.KindPVC,
			Modes: []detection.Mode{ModeVolumePinned}},
		Link:   LinkMounts,
		Effect: podSide(detection.ModePending, detection.ModeUnschedulable),
		Prior:  0.8,
	},
	{
		// Several failing nodes of one zone: an outage, not one
		// broken node.
		Name: "zone-failing",
		Cause: Side{Kind: kube.KindZone, Modes: []detection.Mode{
			ModeMembersFailing}},
		Link: LinkContains, Effect: anything, Prior: 0.6,
		MinCovered: 2,
	},
	{
		// Several failing nodes of one pool: often a bad node image.
		Name: "nodepool-failing",
		Cause: Side{Kind: kube.KindNodePool,
			Modes: []detection.Mode{ModeMembersFailing}},
		Link: LinkContains, Effect: anything, Prior: 0.6,
		MinCovered: 2,
	},
}

// specificRows are the precise rows for the most common failures.
var specificRows = concatRows(nodeRows, workloadRows, clusterRows,
	controlPlaneRows, apiLatencyRows, accessRows, trafficRows,
	nodeLifecycleRows,
	operatorRows, workloadConfigRows, containerRows, calledRows,
	serviceCallRows, policyCallRows, livenessStartRows,
	scalingRows, sharedRows, agentRows, preemptionRows, leaseRows,
	missingServiceRows, initWaitRows)

func concatRows(groups ...[]Row) []Row {
	var out []Row
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

// KindScheduling is the virtual entity of a scheduling constraint,
// named after the blocker the scheduler reported most.
const KindScheduling = rootcause.KindScheduling
