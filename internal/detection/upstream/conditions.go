package upstream

// Condition is one built-in status condition type of a kind.
type Condition struct {
	Kind string
	Type string
	// Trouble is the condition status that means something is wrong:
	// "True" for pressure and failure conditions, "False" for health
	// conditions. Empty means the condition is progress or history and
	// is never trouble by itself.
	Trouble string
	// Reason is the reason that status carries when it is trouble and
	// the condition is only trouble with that reason.
	Reason string
}

// Conditions lists the condition types Kubernetes defines. Sources:
// k8s.io/api (core/v1 pod, node and PVC conditions, apps/v1
// Deployment, batch/v1 Job, autoscaling/v2 HPA conditions) and, for
// CustomResourceDefinition, k8s.io/apiextensions-apiserver
// (CustomResourceDefinitionConditionType).
var Conditions = []Condition{
	{"Deployment", "Available", "False", ""},
	{"Deployment", "Progressing", "False", "ProgressDeadlineExceeded"},
	{"Deployment", "ReplicaFailure", "True", ""},
	{"Node", "Ready", "False", ""},
	{"Node", "MemoryPressure", "True", ""},
	{"Node", "DiskPressure", "True", ""},
	{"Node", "PIDPressure", "True", ""},
	{"Node", "NetworkUnavailable", "True", ""},
	{"Pod", "PodScheduled", "False", ""},
	{"Pod", "Initialized", "False", ""},
	{"Pod", "ContainersReady", "False", ""},
	{"Pod", "Ready", "False", ""},
	{"Pod", "DisruptionTarget", "", ""},
	{"Pod", "PodReadyToStartContainers", "False", ""},
	{"PersistentVolumeClaim", "Resizing", "", ""},
	{"PersistentVolumeClaim", "FileSystemResizePending", "", ""},
	{"PersistentVolumeClaim", "ModifyingVolume", "", ""},
	{"PersistentVolumeClaim", "ControllerResizeError", "True", ""},
	{"PersistentVolumeClaim", "NodeResizeError", "True", ""},
	{"PersistentVolumeClaim", "ModifyVolumeError", "True", ""},
	{"Job", "Complete", "", ""},
	{"Job", "Failed", "True", ""},
	{"Job", "Suspended", "", ""},
	{"Job", "FailureTarget", "True", ""},
	{"Job", "SuccessCriteriaMet", "", ""},
	{"HorizontalPodAutoscaler", "AbleToScale", "False", ""},
	{"HorizontalPodAutoscaler", "ScalingActive", "False", ""},
	{"HorizontalPodAutoscaler", "ScalingLimited", "True", ""},
	{"CustomResourceDefinition", "Established", "False", ""},
	{"CustomResourceDefinition", "NamesAccepted", "False", ""},
	{"CustomResourceDefinition", "NonStructuralSchema", "True", ""},
	{"CustomResourceDefinition", "Terminating", "", ""},
	{"CustomResourceDefinition",
		"KubernetesAPIApprovalPolicyConformant", "False", ""},
}
