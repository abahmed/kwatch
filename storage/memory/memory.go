package memory

import (
	"sync"

	storage "github.com/abahmed/kwatch/storage"
)

type memory struct {
	smap sync.Map
	nmap sync.Map
}

// NewMemory returns new Memory object
func NewMemory() storage.Storage {
	return &memory{
		smap: sync.Map{},
		nmap: sync.Map{},
	}
}

// AddPodContainer attaches container to pod to mark it has an error
func (m *memory) AddPodContainer(namespace, podKey, containerKey string, state *storage.ContainerState) {
	key := m.getKey(namespace, podKey)
	if v, ok := m.smap.Load(key); ok {
		containers := v.(map[string]*storage.ContainerState)
		containers[containerKey] = state

		m.smap.Store(key, containers)
		return
	}
	m.smap.Store(key, map[string]*storage.ContainerState{containerKey: state})
}

// Delete deletes pod with all its containers
func (m *memory) DelPod(namespace, podKey string) {
	key := m.getKey(namespace, podKey)
	m.smap.Delete(key)
}

// DelPodContainer detaches container from pod to mark error is resolved
func (m *memory) DelPodContainer(namespace, podKey, containerKey string) {
	key := m.getKey(namespace, podKey)

	v, ok := m.smap.Load(key)
	if !ok {
		return
	}

	containers := v.(map[string]*storage.ContainerState)
	delete(containers, containerKey)

	m.smap.Store(key, containers)
}

// HasPodContainer checks if container is attached to given pod or not
func (m *memory) HasPodContainer(namespace, podKey, containerKey string) bool {
	key := m.getKey(namespace, podKey)

	v, ok := m.smap.Load(key)
	if !ok {
		return false
	}

	containers := v.(map[string]*storage.ContainerState)
	if _, ok := containers[containerKey]; ok {
		return true
	}

	return false
}

func (m *memory) GetPodContainer(namespace, podKey, containerKey string) *storage.ContainerState {
	key := m.getKey(namespace, podKey)

	v, ok := m.smap.Load(key)
	if !ok {
		return nil
	}

	containers := v.(map[string]*storage.ContainerState)
	if val, ok := containers[containerKey]; ok {
		return val
	}

	return nil
}

func (*memory) getKey(namespace, pod string) string {
	return namespace + "/" + pod
}

// AddNode stores node with key
func (m *memory) AddNode(nodeKey string) {
	m.nmap.Store(nodeKey, true)
}

// HasNode checks if node is stored
func (m *memory) HasNode(nodeKey string) bool {
	_, ok := m.nmap.Load(nodeKey)
	return ok
}

// AddNode deletes node with key
func (m *memory) DelNode(nodeKey string) {
	m.nmap.Delete(nodeKey)
}
