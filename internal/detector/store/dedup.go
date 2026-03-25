package store

import (
	"fmt"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/detector"
)

const (
	DedupKeyPrefix = "dedup:"
	DedupWindow    = 5 * time.Minute
)

type Deduplication struct {
	store  *Store
	events map[string]time.Time
	mu     sync.RWMutex
}

func NewDeduplication(store *Store) *Deduplication {
	d := &Deduplication{
		store:  store,
		events: make(map[string]time.Time),
	}
	d.load()
	return d
}

func (d *Deduplication) Name() string {
	return "Deduplication"
}

func (d *Deduplication) ShouldAlert(input *detector.Input) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := d.getKey(input)
	lastSeen, exists := d.events[key]

	if !exists {
		d.events[key] = time.Now()
		return true
	}

	if time.Since(lastSeen) > DedupWindow {
		d.events[key] = time.Now()
		return true
	}

	return false
}

func (d *Deduplication) Record(input *detector.Input) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := d.getKey(input)
	d.events[key] = time.Now()
	d.save()
}

func (d *Deduplication) getKey(input *detector.Input) string {
	container := ""
	if input.Container != nil {
		container = input.Container.Name
	}
	return fmt.Sprintf("%s/%s/%s/%s", input.EventType, input.Pod.Namespace, input.Pod.Name, container)
}

func (d *Deduplication) load() {
	d.mu.Lock()
	defer d.mu.Unlock()

	var events map[string]time.Time
	err := d.store.Read(DedupKeyPrefix+"events", &events)
	if err != nil || events == nil {
		d.events = make(map[string]time.Time)
		return
	}
	d.events = events
}

func (d *Deduplication) save() {
	if d.store == nil {
		return
	}
	_ = d.store.Write(DedupKeyPrefix+"events", d.events)
}

func (d *Deduplication) Cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	cutoff := time.Now().Add(-DedupWindow)
	for key, t := range d.events {
		if t.Before(cutoff) {
			delete(d.events, key)
		}
	}
	d.save()
}

type DedupStats struct {
	TotalSuppressed int
	TotalSent       int
	Window          time.Duration
}

func (d *Deduplication) GetStats() DedupStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return DedupStats{
		TotalSent: len(d.events),
		Window:    DedupWindow,
	}
}
