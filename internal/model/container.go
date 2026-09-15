package model

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// ContainerContext carries policy findings and optional enrichment data for
// one init or application container. Policy code ignores enrichment fields.
type ContainerContext struct {
	Container        *corev1.ContainerStatus
	Reason           string
	Msg              string
	ExitCode         int32
	Logs             string
	HasRestarts      bool
	LastTerminatedOn time.Time
	State            string
	Status           string
	LastState        *ContainerState
	IsInit           bool
}
