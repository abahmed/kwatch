package store

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/detector"
)

type Store struct {
	volume detector.Volume
	mem    *MemoryStore
	mu     sync.RWMutex
}

func NewStore(volume detector.Volume) *Store {
	s := &Store{
		volume: volume,
		mem:    NewMemoryStore(),
	}
	if volume == nil {
		s.volume = NewMemoryVolume()
	}
	return s
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return nil
}

func (s *Store) Read(key string, dest interface{}) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := s.volume.Read(key)
	if err != nil {
		s.mem.Load(key, dest)
		return err
	}
	return json.Unmarshal(data, dest)
}

func (s *Store) Write(key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	err = s.volume.Write(key, data)
	if err != nil {
		return err
	}

	s.mem.Store(key, value)
	return nil
}

func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.volume.Delete(key)
	if err != nil {
		return err
	}

	s.mem.Delete(key)
	return nil
}

func (s *Store) Sync() error {
	return nil
}

type MemoryStore struct {
	data map[string]interface{}
	mu   sync.RWMutex
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]interface{}),
	}
}

func (m *MemoryStore) Store(key string, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
}

func (m *MemoryStore) Load(key string, dest interface{}) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.data[key]
	if !ok {
		return false
	}
	data, err := json.Marshal(val)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, dest) == nil
}

func (m *MemoryStore) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
}

func (m *MemoryStore) Get(key string) (interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.data[key]
	return val, ok
}

type MemoryVolume struct {
	data map[string][]byte
	mu   sync.RWMutex
}

func NewMemoryVolume() *MemoryVolume {
	return &MemoryVolume{
		data: make(map[string][]byte),
	}
}

func (m *MemoryVolume) Read(key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	return data, nil
}

func (m *MemoryVolume) Write(key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = data
	return nil
}

func (m *MemoryVolume) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

type StoreConfig struct {
	SyncInterval time.Duration
	MaxMemoryAge time.Duration
}

var DefaultStoreConfig = StoreConfig{
	SyncInterval: 30 * time.Second,
	MaxMemoryAge: 5 * time.Minute,
}

func (s *Store) StartSync(cfg StoreConfig) {
	go func() {
		ticker := time.NewTicker(cfg.SyncInterval)
		defer ticker.Stop()
		for range ticker.C {
			s.Sync()
		}
	}()
}

func (s *Store) NewAggregation(window time.Duration) *Aggregation {
	return NewAggregation(s, window)
}

func (s *Store) NewDeduplication() *Deduplication {
	return NewDeduplication(s)
}

func (s *Store) NewClusterStore(threshold int, window time.Duration) *ClusterStore {
	return NewClusterStore(s, threshold, window)
}

func (s *Store) NewPVCState() *PVCState {
	return NewPVCState(s)
}
