package detection

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
	eventReason("HostPortConflict", Critical),
	eventReason("HostNetworkNotSupported", Critical),
	eventReason("FailedPodSandBoxStatus", Warning),
	eventReason("FailedSync", Warning),
	eventReason("UnsupportedMountOption", Warning),
	eventReason("FailedUnprepareDynamicResources", Warning),
	eventReason("ExceededGracePeriod", Warning),
	eventReason("ProbeWarning", Info),
	// Node and kubelet.
	eventReason("KubeletSetupFailed", Critical),
	eventReason("Rebooted", Warning),
	eventReason("InvalidDiskCapacity", Warning),
	eventReason("FailedNodeAllocatableEnforcement", Warning),
	// The node conditions and the OOM-killed container already show
	// these.
	stateReason("NodeHasInsufficientMemory"),
	stateReason("NodeHasDiskPressure"),
	stateReason("NodeHasInsufficientPID"),
	stateReason("SystemOOM"),
	// Controllers.
	eventReason("FailedDelete", Warning),
	eventReason("FailedPlacement", Warning),
	eventReason("SelectingAll", Warning),
	eventReason("FailedUpdateStatus", Warning),
	eventReason("FailedNeedsStart", Warning),
	eventReason("PodFailurePolicy", Info),
	eventReason("UnexpectedJob", Info),
	eventReason("MissingJob", Info),
	eventReason("UnmanagedPods", Info),
	eventReason("DeploymentRollbackRevisionNotFound", Warning),
	eventReason("DeploymentRollbackTemplateUnchanged", Info),
	// The HPA's own conditions carry these reasons, and the autoscaler
	// detector reads them there.
	stateReason("InvalidSchedule"),
	stateReason("FailedRescale"), stateReason("FailedConvertHPA"),
	stateReason("FailedGetExternalMetric"),
	stateReason("FailedGetObjectMetric"),
	stateReason("FailedGetPodsMetric"),
	stateReason("FailedGetContainerResourceMetric"),
	stateReason("InvalidMetricSourceType"),
	stateReason("InvalidSelector"), stateReason("SelectorRequired"),
	stateReason("AmbiguousSelector"),
	// Storage.
	eventReason("FailedDetachVolume", Warning),
	eventReason("FileSystemResizeFailed", Warning),
	eventReason("ProvisioningCleanupFailed", Warning),
	eventReason("VolumeFailedRecycle", Warning),
	eventReason("VolumeFailedDelete", Warning),
	eventReason("ClaimLost", Critical),
	eventReason("VolumeMismatch", Info),
	eventReason("WarnAlreadyMountedVolume", Info),
	// Endpoints, load balancers and the rest of the control plane. The
	// Service detectors read the endpoint and load balancer failures
	// from the Service itself, the namespace and CSR detectors from
	// their conditions.
	stateReason("FailedToCreateEndpoint"),
	stateReason("FailedToCreateEndpointSlice"),
	stateReason("FailedToDeleteEndpoint"),
	stateReason("UpdateLoadBalancerFailed"),
	stateReason("DeleteLoadBalancerFailed"),
	stateReason("UnAvailableLoadBalancer"),
	stateReason("SyncLoadBalancerFailed"),
	stateReason("NamespaceDeletionContentFailure"),
	eventReason("FailedToCreateRoute", Warning),
}
