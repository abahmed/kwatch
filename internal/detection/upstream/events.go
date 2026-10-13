package upstream

// Event is one reason a Kubernetes component puts on an Event.
type Event struct {
	Reason string
	// Warning is true for an event the component records as type
	// Warning. Normal events are never ingested by kwatch.
	Warning bool
}

// w and n build a Warning and a Normal entry.
func w(reason string) Event { return Event{Reason: reason, Warning: true} }
func n(reason string) Event { return Event{Reason: reason} }

// Events lists the event reasons of the kubelet, the scheduler and the
// controller manager, grouped by the source file that defines them.
var Events = concat(kubeletEvents, nodeEvents, schedulerEvents,
	workloadEvents, storageEvents, networkEvents)

func concat(groups ...[]Event) []Event {
	var out []Event
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// kubeletEvents: pkg/kubelet/events/event.go (pod and container
// lifecycle, images, probes, hooks, sandbox).
var kubeletEvents = []Event{
	n("Created"), n("Started"), n("Killing"), n("Pulling"), n("Pulled"),
	n("SandboxChanged"), n("Preempting"), w("Failed"),
	w("BackOff"), w("ExceededGracePeriod"), w("FailedKillPod"),
	w("FailedCreatePodContainer"), w("NetworkNotReady"), w("InspectFailed"),
	w("ErrImageNeverPull"), w("FailedCreatePodSandBox"),
	w("FailedPodSandBoxStatus"), w("Unhealthy"), w("ProbeWarning"),
	w("FailedPostStartHook"), w("FailedPreStopHook"), w("FailedSync"),
	w("FailedValidation"), w("HostNetworkNotSupported"), w("HostPortConflict"),
	w("FailedPrepareDynamicResources"), w("FailedUnprepareDynamicResources"),
	w("UnsupportedMountOption"),
}

// nodeEvents: pkg/kubelet/events/event.go and kubelet_node_status.go
// (node status, resources, GC, eviction, reboot), plus the node
// lifecycle controller (pkg/controller/nodelifecycle).
var nodeEvents = []Event{
	n("NodeReady"), n("NodeSchedulable"), n("Starting"),
	n("NodeHasSufficientMemory"), n("NodeHasNoDiskPressure"),
	n("NodeHasSufficientPID"), n("NodeAllocatableEnforced"),
	n("RegisteredNode"), n("RemovingNode"), n("DeletingNode"),
	n("DeletingAllPods"), n("TerminatingEvictedPod"), n("TaintManagerEviction"),
	w("NodeNotReady"), w("NodeNotSchedulable"), w("KubeletSetupFailed"),
	w("NodeHasInsufficientMemory"), w("NodeHasDiskPressure"),
	w("NodeHasInsufficientPID"), w("EvictionThresholdMet"), w("Rebooted"),
	w("SystemOOM"), w("InvalidDiskCapacity"), w("FreeDiskSpaceFailed"),
	w("ImageGCFailed"), w("ContainerGCFailed"),
	w("FailedNodeAllocatableEnforcement"),
}

// schedulerEvents: pkg/scheduler/schedule_one.go and the preemption
// plugin.
var schedulerEvents = []Event{
	n("Scheduled"), n("Preempted"), w("FailedScheduling"),
}

// workloadEvents: pkg/controller/{replicaset,deployment,daemon,job,
// cronjob,statefulset,podautoscaler,disruption,history}.
var workloadEvents = []Event{
	n("SuccessfulCreate"), n("SuccessfulDelete"), n("ScalingReplicaSet"),
	n("DeploymentRollback"), n("SuccessfulRescale"), n("Completed"),
	n("SawCompletedJob"), n("Suspended"), n("Resumed"),
	n("RecreatingFailedPod"), n("RecreatingTerminatedPod"),
	n("SuccessfulAttachVolume"), n("SuccessfulMountVolume"), w("FailedCreate"),
	w("FailedDelete"), w("FailedDaemonPod"), w("FailedPlacement"),
	w("SelectingAll"), w("BackoffLimitExceeded"), w("DeadlineExceeded"),
	w("PodFailurePolicy"), w("TooManyMissedTimes"), w("FailedNeedsStart"),
	w("InvalidSchedule"), w("UnexpectedJob"), w("MissingJob"),
	w("DeploymentRollbackRevisionNotFound"),
	w("DeploymentRollbackTemplateUnchanged"), w("FailedRescale"),
	w("FailedGetScale"), w("FailedGetResourceMetric"),
	w("FailedGetExternalMetric"), w("FailedGetObjectMetric"),
	w("FailedGetPodsMetric"), w("FailedGetContainerResourceMetric"),
	w("InvalidMetricSourceType"), w("FailedComputeMetricsReplicas"),
	w("FailedUpdateStatus"), w("FailedConvertHPA"), w("FailedGetMetrics"),
	w("NoPods"), w("CalculateExpectedPodCountFailed"), w("UnmanagedPods"),
	w("InvalidSelector"), w("SelectorRequired"), w("AmbiguousSelector"),
	w("FailedUpdateScale"),
}

// storageEvents: pkg/controller/volume/{persistentvolume,attachdetach,
// expand}, pkg/volume (FailedMount and friends) and the kubelet volume
// manager.
var storageEvents = []Event{
	n("ExternalProvisioning"), n("ProvisioningSucceeded"),
	n("WaitForFirstConsumer"), n("VolumeRecycled"), n("VolumeDelete"),
	n("RecyclerPod"), n("VolumeResizeSuccessful"),
	n("FileSystemResizeSuccessful"), n("AlreadyMountedVolume"),
	w("FailedMount"), w("FailedAttachVolume"), w("FailedDetachVolume"),
	w("FailedMapVolume"), w("WarnAlreadyMountedVolume"),
	w("VolumeResizeFailed"), w("FileSystemResizeFailed"),
	w("ProvisioningFailed"), w("ProvisioningCleanupFailed"), w("FailedBinding"),
	w("VolumeMismatch"), w("VolumeFailedRecycle"), w("VolumeFailedDelete"),
	w("ClaimLost"),
}

// networkEvents: pkg/controller/{endpoint,endpointslice,service,route,
// certificates}, the API server webhook dispatcher and the
// namespace controller.
var networkEvents = []Event{
	n("EnsuringLoadBalancer"), n("EnsuredLoadBalancer"),
	n("UpdatedLoadBalancer"), n("DeletingLoadBalancer"),
	n("DeletedLoadBalancer"), w("FailedToCreateEndpoint"),
	w("FailedToUpdateEndpoint"), w("FailedToUpdateEndpointSlices"),
	w("FailedToCreateEndpointSlice"), w("FailedToDeleteEndpoint"),
	w("SyncLoadBalancerFailed"), w("DeleteLoadBalancerFailed"),
	w("UnAvailableLoadBalancer"), w("UpdateLoadBalancerFailed"),
	w("FailedToCreateRoute"), w("FailedCallingWebhook"),
	w("NamespaceDeletionContentFailure"),
}
