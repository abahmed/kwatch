package detection

import "github.com/abahmed/kwatch/internal/inventory/kube"

// upstreamReasons are the Kubernetes Warning event reasons that have no
// finding of their own beyond the event: the kubelet, node, controller
// and storage events the upstream snapshot lists (detection/upstream).
// Each has the severity of what it means, or is marked shown when a
// detector already reads the same failure from the object itself, so
// nothing is reported twice. Info events are quoted for evidence and
// reach the digest at most.
var upstreamReasons = []ReasonInfo{
	// Pod start: the kubelet cannot create the pod's cgroup, bind its
	// ports or set up its sandbox, so the pod cannot run.
	eventReason("FailedCreatePodContainer", Critical),
	eventReason("FailedPodSandBoxStatus", Warning),
	eventReason("FailedSync", Warning),
	eventReason("ExceededGracePeriod", Warning),
	eventReason("AlreadyMountedVolume", Info),
	eventReason("ProbeWarning", Info),
	// Node and kubelet.
	eventReason("KubeletSetupFailed", Critical),
	// The kubelet writes these notices on healthy starts: a node that
	// booted (Rebooted), cadvisor not yet ready (InvalidDiskCapacity),
	// a long DNS search line (DNSConfigForming, CheckLimitsForResolvConf).
	// Pods run fine, so they reach the digest at most.
	eventReason("Rebooted", Info),
	eventReason("InvalidDiskCapacity", Info),
	eventReason("FailedNodeAllocatableEnforcement", Warning),
	// The node conditions and the OOM-killed container already show
	// these.
	stateReason("NodeHasInsufficientMemory", kube.KindNode),
	stateReason("NodeHasDiskPressure", kube.KindNode),
	stateReason("NodeHasInsufficientPID", kube.KindNode),
	stateReason("SystemOOM", kube.KindNode),
	// Controllers.
	eventReason("FailedDelete", Warning),
	eventReason("SelectingAll", Warning),
	eventReason("FailedUpdateStatus", Warning),
	eventReason("PodFailurePolicy", Info),
	eventReason("UnexpectedJob", Info),
	eventReason("MissingJob", Info),
	eventReason("UnmanagedPods", Info),
	eventReason("DeploymentRollbackRevisionNotFound", Warning),
	eventReason("DeploymentRollbackTemplateUnchanged", Info),
	// The HPA's own conditions carry these reasons, and the autoscaler
	// detector reads them there.
	stateReason("InvalidSchedule", kube.KindCronJob),
	stateReason("FailedRescale", kube.KindHPA),
	stateReason("FailedGetExternalMetric", kube.KindHPA),
	stateReason("FailedGetObjectMetric", kube.KindHPA),
	stateReason("FailedGetPodsMetric", kube.KindHPA),
	stateReason("FailedGetContainerResourceMetric", kube.KindHPA),
	stateReason("InvalidMetricSourceType", kube.KindHPA),
	stateReason("InvalidSelector", kube.KindHPA),
	stateReason("SelectorRequired", kube.KindHPA),
	stateReason("AmbiguousSelector", kube.KindHPA),
	// Storage.
	eventReason("FileSystemResizeFailed", Warning),
	eventReason("ProvisioningCleanupFailed", Warning),
	eventReason("VolumeFailedRecycle", Warning),
	eventReason("VolumeFailedDelete", Warning),
	eventReason("ClaimLost", Critical),
	eventReason("VolumeMismatch", Info),
	// Endpoints, load balancers and the rest of the control plane. The
	// Service detectors read the endpoint and load balancer failures
	// from the Service itself, the namespace and CSR detectors from
	// their conditions.
	{Reason: "FailedToCreateEndpoint"},
	stateReason("UpdateLoadBalancerFailed", kube.KindService),
	stateReason("DeleteLoadBalancerFailed", kube.KindService),
	stateReason("UnAvailableLoadBalancer", kube.KindService),
	stateReason("SyncLoadBalancerFailed", kube.KindService),
	eventReason("FailedToCreateRoute", Warning),
	// Kubelet notices added since the first snapshot: advisory ones
	// are Info, a mount, resize or certificate that failed is not.
	eventReason("CgroupV1", Info),
	eventReason("CheckLimitsForResolvConf", Info),
	eventReason("DNSConfigForming", Info),
	eventReason("MissingClusterDNS", Warning),
	eventReason("FailedMountOnFilesystemMismatch", Critical),
	eventReason("FailedNodeDeclaredFeaturesCheck", Warning),
	eventReason("FailedToRetrieveImagePullSecret", Warning),
	eventReason("HostIPsIPFamilyMismatch", Warning),
	eventReason("InvalidImageID", Warning),
	eventReason("LifecycleHTTPFallback", Info),
	eventReason("PossibleMemoryBackedVolumesOnDisk", Info),
	eventReason("VolumePermissionChangeInProgress", Info),
	eventReason("ResizeDeferred", Info),
	eventReason("ResizeInfeasible", Warning),
	eventReason("ResizeError", Warning),
	eventReason("CertificateExpired", Warning),
	eventReason("CertificateOverdueForRefresh", Warning),
	eventReason("Denied", Warning),
	// More controller events. The Job, Deployment and CronJob ones are
	// the reasons of conditions and specs the workload detectors read.
	stateReason("FailedIndexes", kube.KindJob),
	stateReason("MaxFailedIndexesExceeded", kube.KindJob),
	stateReason("ReplicaSetCreateError", kube.KindDeployment),
	// The PodDisruptionBudget detector finds overlapping budgets from
	// the budgets themselves.
	stateReason("MultiplePodDisruptionBudgets", kube.KindPod),
	stateReason("UnParseableCronJobSchedule", kube.KindCronJob),
	stateReason("UnparseableSchedule", kube.KindCronJob),
	stateReason("UnsupportedSchedule", kube.KindCronJob),
	eventReason("ClaimMisbound", Warning),
	eventReason("ConflictingController", Warning),
	eventReason("DeletingNodeFailed", Warning),
	eventReason("ExternalExpanding", Info),
	eventReason("FailedGet", Warning),
	eventReason("FailedUpdate", Warning),
	eventReason("FailedResourceClaimCreation", Warning),
	eventReason("FailedToListEndpointSlices", Warning),
	eventReason("FailedToListPods", Warning),
	eventReason("FailedWorkloadCompilation", Warning),
	eventReason("FailedWorkloadCreate", Warning),
	eventReason("InvalidIPAddress", Warning),
	eventReason("KubernetesServiceCIDRError", Warning),
	eventReason("MissSchedule", Warning),
	eventReason("MultipleSELinuxLabels", Warning),
	eventReason("NotDeleted", Warning),
	eventReason("OwnerRefInvalidNamespace", Warning),
	eventReason("RecyclerPod", Info),
	eventReason("TooManyAddressesToMirror", Warning),
	eventReason("TooManySucceededPods", Warning),
	eventReason("UnknownCompletionMode", Warning),
	eventReason("UnknownStrategy", Warning),
	eventReason("UnknownTimeZone", Warning),
	eventReason("VolumeUnknownReclaimPolicy", Warning),
}
