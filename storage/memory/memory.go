package memory

import (
	"sync"

	storage "github.com/abahmed/kwatch/storage"
)

type memory struct {
	smap sync.Map
}

// NewMemory returns new Memory object
func NewMemory() storage.Storage {
	return &memory{
		smap: sync.Map{},
	}
}

// AddPodContainer attaches container to pod to mark it has an error
func (m *memory) AddPodContainer(podKey, containerKey string) {
	if v, ok := m.smap.Load(podKey); ok {
		containers := v.(map[string]bool)
		containers[containerKey] = true

		m.smap.Store(podKey, containers)
		return
	}
	m.smap.Store(podKey, map[string]bool{containerKey: true})
}

// Delete deletes pod with all its containers
func (m *memory) DelPod(key string) {
	m.smap.Delete(key)
}

// DelPodContainer detaches container from pod to mark error is resolved
func (m *memory) DelPodContainer(podKey, containerKey string) {
	v, ok := m.smap.Load(podKey)
	if !ok {
		return
	}

	containers := v.(map[string]bool)
	delete(containers, containerKey)

	m.smap.Store(podKey, containers)
}

// HasPodContainer checks if container is attached to given pod or not
func (m *memory) HasPodContainer(podKey, containerKey string) bool {
	v, ok := m.smap.Load(podKey)
	if !ok {
		return false
	}

	containers := v.(map[string]bool)
	if _, ok := containers[containerKey]; ok {
		return true
	}

	return false
}
