package storage

import "time"

type ContainerState struct {
	RestartCount     int32
	LastTerminatedOn time.Time
	Reason           string
	Msg              string
	ExitCode         int32
	Status           string
	Reported         bool
}

// Storage interface
type Storage interface {
	AddPodContainer(namespace, podKey, containerKey string, state *ContainerState)
	DelPodContainer(namespace, podKey, containerKey string)
	DelPod(namespace, podKey string)
	HasPodContainer(namespace, podKey, containerKey string) bool
	GetPodContainer(namespace, podKey, containerKey string) *ContainerState

	AddNode(nodeKey string)
	HasNode(nodeKey string) bool
	DelNode(nodeKey string)
}
