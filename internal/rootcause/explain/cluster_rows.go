package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Rows for shared cluster services.

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
