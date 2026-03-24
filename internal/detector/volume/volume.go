package volume

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Config holds volume configuration
type Config struct {
	BasePath     string
	SyncInterval time.Duration
}

// Volume implements file-based storage
type Volume struct {
	basePath     string
	syncInterval time.Duration
	mu           sync.RWMutex
	memoryCache  map[string][]byte
}

// New creates a new volume storage
func New(config *Config) (*Volume, error) {
	basePath := config.BasePath
	if basePath == "" {
		basePath = os.Getenv("DATA_PATH")
		if basePath == "" {
			basePath = "/data"
		}
	}

	v := &Volume{
		basePath:     basePath,
		syncInterval: config.SyncInterval,
		memoryCache:  make(map[string][]byte),
	}

	if err := os.MkdirAll(basePath, 0755); err != nil {
		logrus.Warnf("Failed to create volume directory: %v, using memory only", err)
	}

	v.loadAll()

	go v.periodicSync()

	return v, nil
}

// Read reads data from volume
func (v *Volume) Read(key string) ([]byte, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if data, ok := v.memoryCache[key]; ok {
		return data, nil
	}

	path := filepath.Join(v.basePath, key+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Write writes data to volume
func (v *Volume) Write(key string, data []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.memoryCache[key] = data

	path := filepath.Join(v.basePath, key+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		logrus.Debugf("Failed to write to volume: %v, using memory only", err)
	}

	return nil
}

// Delete deletes data from volume
func (v *Volume) Delete(key string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	delete(v.memoryCache, key)

	path := filepath.Join(v.basePath, key+".json")
	if err := os.Remove(path); err != nil {
		logrus.Debugf("Failed to delete from volume: %v", err)
	}

	return nil
}

func (v *Volume) loadAll() {
	files, err := os.ReadDir(v.basePath)
	if err != nil {
		return
	}

	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			key := f.Name()[:len(f.Name())-5]
			data, err := os.ReadFile(filepath.Join(v.basePath, f.Name()))
			if err == nil {
				v.memoryCache[key] = data
			}
		}
	}
}

func (v *Volume) periodicSync() {
	ticker := time.NewTicker(v.syncInterval)
	for range ticker.C {
		v.mu.RLock()
		for key, data := range v.memoryCache {
			path := filepath.Join(v.basePath, key+".json")
			_ = os.WriteFile(path, data, 0644)
		}
		v.mu.RUnlock()
	}
}

// Metadata holds volume metadata
type Metadata struct {
	LastSync  time.Time `json:"lastSync"`
	LastSize  int64     `json:"lastSize"`
	FileCount int       `json:"fileCount"`
	LastCheck time.Time `json:"lastCheck"`
}

// GetMetadata returns volume metadata
func (v *Volume) GetMetadata() Metadata {
	v.mu.RLock()
	defer v.mu.RUnlock()

	var size int64
	count := 0
	for _, data := range v.memoryCache {
		size += int64(len(data))
		count++
	}

	return Metadata{
		LastSync:  time.Now(),
		LastSize:  size,
		FileCount: count,
		LastCheck: time.Now(),
	}
}

// PVCState represents PVC state
type PVCState struct {
	Name          string    `json:"name"`
	Namespace     string    `json:"namespace"`
	Attached      bool      `json:"attached"`
	UsedBytes     int64     `json:"usedBytes"`
	CapacityBytes int64     `json:"capacityBytes"`
	UsagePercent  float64   `json:"usagePercent"`
	LastSeen      time.Time `json:"lastSeen"`
}

// PVCStates holds all PVC states
type PVCStates struct {
	States        map[string]map[string]PVCState `json:"pvcStates"`
	MonitoredPVcs []string                       `json:"monitoredPVcs"`
}

// SavePVCState saves PVC state
func (v *Volume) SavePVCState(name, namespace string, state PVCState) error {
	data, _ := v.Read("pvc_usage")

	var pvcStates PVCStates
	if err := json.Unmarshal(data, &pvcStates); err != nil {
		pvcStates = PVCStates{States: make(map[string]map[string]PVCState)}
	}

	if pvcStates.States == nil {
		pvcStates.States = make(map[string]map[string]PVCState)
	}
	if pvcStates.States[namespace] == nil {
		pvcStates.States[namespace] = make(map[string]PVCState)
	}

	pvcStates.States[namespace][name] = state

	newData, _ := json.Marshal(pvcStates)
	return v.Write("pvc_usage", newData)
}

// GetPVCState gets PVC state
func (v *Volume) GetPVCState(name, namespace string) (PVCState, bool) {
	data, err := v.Read("pvc_usage")
	if err != nil {
		return PVCState{}, false
	}

	var pvcStates PVCStates
	if err := json.Unmarshal(data, &pvcStates); err != nil {
		return PVCState{}, false
	}

	if pvcStates.States == nil || pvcStates.States[namespace] == nil {
		return PVCState{}, false
	}

	state, ok := pvcStates.States[namespace][name]
	return state, ok
}
