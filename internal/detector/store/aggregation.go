package store

import (
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/detector"
)

const AggregationKeyPrefix = "agg:"

type Aggregation struct {
	store  *Store
	counts map[string]*PodAggregation
	window time.Duration
	mu     sync.RWMutex
}

type PodAggregation struct {
	PodKey    string
	Count     int
	Events    []AggregationEvent
	FirstSeen time.Time
	LastSeen  time.Time
	Reason    string
	Message   string
}

type AggregationEvent struct {
	Timestamp time.Time
	Reason    string
	Message   string
	Container string
}

func NewAggregation(store *Store, window time.Duration) *Aggregation {
	a := &Aggregation{
		store:  store,
		counts: make(map[string]*PodAggregation),
		window: window,
	}
	if window == 0 {
		a.window = 10 * time.Minute
	}
	a.load()
	return a
}

func (a *Aggregation) Name() string {
	return "Aggregation"
}

func (a *Aggregation) ShouldAggregate(input *detector.Input) bool {
	if input.Pod == nil {
		return false
	}

	key := a.getKey(input)
	a.mu.Lock()
	defer a.mu.Unlock()

	agg, exists := a.counts[key]
	if !exists {
		a.counts[key] = &PodAggregation{
			PodKey:    key,
			Count:     1,
			FirstSeen: time.Now(),
			LastSeen:  time.Now(),
			Reason:    input.Reason,
			Message:   input.Message,
		}
		a.counts[key].Events = append(a.counts[key].Events, AggregationEvent{
			Timestamp: time.Now(),
			Reason:    input.Reason,
			Message:   input.Message,
			Container: getContainerName(input),
		})
		a.save()
		return false
	}

	agg.Count++
	agg.LastSeen = time.Now()
	agg.Events = append(agg.Events, AggregationEvent{
		Timestamp: time.Now(),
		Reason:    input.Reason,
		Message:   input.Message,
		Container: getContainerName(input),
	})

	a.save()
	return agg.Count >= 3
}

func (a *Aggregation) GetAggregation(key string) *PodAggregation {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.counts[key]
}

func (a *Aggregation) GetSummary(key string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	agg, exists := a.counts[key]
	if !exists || agg.Count == 0 {
		return ""
	}

	return agg.GetSummary()
}

func (p *PodAggregation) GetSummary() string {
	if p.Count == 0 {
		return ""
	}
	return p.Message
}

func (a *Aggregation) getKey(input *detector.Input) string {
	container := ""
	if input.Container != nil {
		container = input.Container.Name
	}
	return input.Pod.Namespace + "/" + input.Pod.Name + "/" + container
}

func (a *Aggregation) load() {
	a.mu.Lock()
	defer a.mu.Unlock()

	var counts map[string]*PodAggregation
	err := a.store.Read(AggregationKeyPrefix+"counts", &counts)
	if err != nil || counts == nil {
		a.counts = make(map[string]*PodAggregation)
		return
	}
	a.counts = counts
}

func (a *Aggregation) save() {
	if a.store == nil {
		return
	}
	_ = a.store.Write(AggregationKeyPrefix+"counts", a.counts)
}

func (a *Aggregation) Cleanup() {
	a.mu.Lock()
	defer a.mu.Unlock()

	cutoff := time.Now().Add(-a.window)
	for key, agg := range a.counts {
		if agg.LastSeen.Before(cutoff) {
			delete(a.counts, key)
		}
	}
	a.save()
}

func (a *Aggregation) GetStats() AggregationStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return AggregationStats{
		TotalAggregated: len(a.counts),
		Window:          a.window,
	}
}

type AggregationStats struct {
	TotalAggregated int
	Window          time.Duration
}

func getContainerName(input *detector.Input) string {
	if input.Container == nil {
		return ""
	}
	return input.Container.Name
}
