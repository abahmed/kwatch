package event

import "github.com/abahmed/kwatch/internal/model"

const PodLineageAnnotation = "kwatch.abahmed.dev/lineage-id"

// Signal is a structured representation of an incident source, designed
// to replace the repetitive event.Event building across handler files.
type Signal struct {
	// "Deployment", "Job", "CronJob", "DaemonSet", "HPA", "Node", "Pod", "PVC"
	Kind      string
	Namespace string
	Owner     string // owner/name of the parent resource
	// "deployment", "job", "cronjob", "daemonset", "hpa", "node", "pod", "pvc"
	Resource        string
	Reason          string
	Message         string
	NodeName        string
	Container       string
	Image           string
	RestartCount    int32
	Severity        model.Severity
	Logs            string
	Events          string
	PodName         string // specific pod (empty for owner-level signals)
	PodUID          string // UID of the concrete Pod instance
	PodLineageID    string // explicit stable lineage for ownerless Pods
	PodGenerateName string // evidence only; not an identity
	Hint            string
	Facts           model.Facts // structured details behind Hint
	OwnerKind       string      // "Deployment", "StatefulSet", etc.
	Labels          map[string]string
	ContainerState  *model.ContainerState // optional pre-built container state
}
