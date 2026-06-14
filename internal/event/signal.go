package event

import "github.com/abahmed/kwatch/internal/model"

// Signal is a structured representation of an incident source, designed
// to replace the repetitive event.Event building across handler files.
type Signal struct {
	Kind           string // "Deployment", "Job", "CronJob", "DaemonSet", "HPA", "Node", "Pod", "PVC"
	Namespace      string
	Owner          string // owner/name of the parent resource
	Resource       string // "deployment", "job", "cronjob", "daemonset", "hpa", "node", "pod", "pvc"
	Reason         string
	Message        string
	NodeName       string
	Container      string
	RestartCount   int32
	Severity       string
	Logs           string
	Events         string
	IncludeEvents  bool
	IncludeLogs    bool
	PodName        string // specific pod (empty for owner-level signals)
	Hint           string
	OwnerKind      string // "Deployment", "StatefulSet", etc.
	Labels         map[string]string
	ContainerState *model.ContainerState // optional pre-built container state
}
