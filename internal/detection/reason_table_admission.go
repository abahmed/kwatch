package detection

// admissionReasons are the reasons the kubelet rejects a pod with at
// admission. The kubelet records each as a Warning event and as the
// status reason of the pod, which it fails, so the Failed pod already
// shows them (detection/upstream: PodReasons).
var admissionReasons = []ReasonInfo{
	stateReason("OutOfcpu"), stateReason("OutOfmemory"),
	stateReason("OutOfpods"), stateReason("OutOfephemeral-storage"),
	stateReason("NodeAffinity"), stateReason("NodeName"),
	stateReason("NodePorts"), stateReason("TaintToleration"),
	stateReason("PodOSNotSupported"),
	stateReason("PodOSSelectorNodeLabelDoesNotMatch"),
	stateReason("TopologyAffinityError"), stateReason("SysctlForbidden"),
	stateReason("InvalidNodeInfo"), stateReason("UnknownReason"),
	stateReason("UnexpectedAdmissionError"),
	stateReason("UnexpectedPredicateFailureType"),
	stateReason("InitContainerRestartPolicyForbidden"),
	stateReason("SupplementalGroupsPolicyNotSupported"),
	stateReason("AppArmor"), stateReason("PodFeatureUnsupported"),
	stateReason("PodLevelResourcesNotSupported"),
	stateReason("NodeShutdown"),
	stateReason("VolumeAttachmentLimitExceeded"),
}
