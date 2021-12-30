package storage

// Storage interface
type Storage interface {
	AddPodContainer(podKey, containerKey string)
	DelPodContainer(podKey, containerKey string)
	DelPod(podKey string)
	HasPodContainer(podKey, containerKey string) bool
}
