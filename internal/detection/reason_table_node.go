package detection

import "github.com/abahmed/kwatch/internal/detection/reasons"

// nodeReasons is the node, resource-usage and control-plane reasons.
var nodeReasons = []ReasonInfo{
	reason(reasons.NodeNotReady, ModeNotReady).shown(),
	reason(reasons.NodeDraining, ModeDraining),
	reason(reasons.NotReady, ModeNotReady),
	reason(reasons.MemoryPressure, ModeMemoryPressure),
	reason(reasons.NodeMemoryPressure, ModeMemoryPressure),
	reason(reasons.DiskPressure, ModeDiskPressure),
	reason(reasons.PIDPressure, ModePIDPressure),
	reason(reasons.NetworkUnavailable, ModeNetworkUnavailable),
	// One mode per resource: severity says how full it is, and the
	// Critical reasons, which only persisted findings carry, share it.
	reason(reasons.NodeFilesystemHigh, ModeFilesystemUsage),
	reason(reasons.NodeFilesystemCritical, ModeFilesystemUsage),
	reason(reasons.NodeInodesHigh, ModeInodesUsage),
	reason(reasons.NodeInodesCritical, ModeInodesUsage),
	reason(reasons.ContainerMemoryHigh, ModeMemoryHigh),
	reason(reasons.ContainerCPUHigh, ModeCPUHigh).digest(),
	reason(reasons.ContainerEphemeralStorageHigh, ModeEphemeralStorageHigh),
	reason(reasons.ActiveProbeFailure, ModeActiveProbe),
	reason(reasons.ContainerCPUThrottled, ModeCPUThrottled).digest(),
	// A node whose workloads stall on CPU, memory or disk is under
	// strain, not failing: when a pod on it fails, that pod is the news.
	reason(reasons.NodePSIHigh, ModePressureStall).digest(),
	reason(reasons.NodeNetworkErrors, ModeNetworkErrors),
	reason(reasons.NodeRuntimeErrors, ModeRuntimeErrors),
	// An overcommitted node is a risk until something is killed; the
	// kill is the incident, and this is its cause.
	reason(reasons.NodeMemoryOvercommitted, ModeMemoryOvercommitted).digest(),
	reason(reasons.TLSCertExpired, ModeCertExpired),
	reason(reasons.APIServerUnavailable, ModeUnavailableAPIServer),
	reason(reasons.APIServerLatency, ModeLatencyAPIServer),
	reason(reasons.APIServerWritesSlow, ModeLatencyWrites),
	reason(reasons.APIServerReadsSlow, ModeLatencyReads),
	reason(reasons.APIServerThrottling, ModeThrottlingAPIServer),
	reason(reasons.EtcdDatabaseLarge, ModeEtcdLarge),
	reason(reasons.StorageObjectsHigh, ModeObjectsHigh),
	reason(reasons.SchedulerUnavailable, ModeUnavailableScheduler),
	reason(reasons.ControllerManagerUnavailable, ModeUnavailableControllerManager),
	reason(reasons.EtcdUnavailable, ModeUnavailableEtcd),
	reason(reasons.CoreDNSUnavailable, ModeUnavailableCoreDNS),
	reason(reasons.ActiveProbeLatency, ModeActiveProbeLatency),
	reason(reasons.ClusterVersionSkew, ModeClusterVersionSkew),
	reason(reasons.VolumeAttachmentFailure, ModeAttachFailed),
	reason(reasons.VolumeSnapshotFailure, ModeSnapshotFailed),
}
