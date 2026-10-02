package kube

import (
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// growthSamples bounds the usage history per volume: two hours at one
// sample per minute.
const (
	growthSamples    = 120
	minGrowthSamples = 10
	// maxFillETA bounds the reported fill time. A slower fill is not
	// news, and converting a larger estimate to a Duration overflows.
	maxFillETA = 30 * 24 * time.Hour
)

type usageSample struct {
	at   time.Time
	used float64
}

// growthTracker estimates when a volume fills from its recent usage using
// a least-squares slope. Shrinking or flat usage has no fill time.
type growthTracker struct {
	mu      sync.Mutex
	samples map[inventory.EntityID][]usageSample
}

func newGrowthTracker() *growthTracker {
	return &growthTracker{samples: map[inventory.EntityID][]usageSample{}}
}

func (g *growthTracker) observe(
	id inventory.EntityID, at time.Time, used, capacity uint64,
) (time.Duration, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	samples := g.samples[id]
	samples = append(samples, usageSample{at: at, used: float64(used)})
	if len(samples) > growthSamples {
		samples = samples[len(samples)-growthSamples:]
	}
	g.samples[id] = samples
	if len(samples) < minGrowthSamples {
		return 0, false
	}
	slope := bytesPerSecond(samples)
	if slope <= 0 {
		return 0, false
	}
	remaining := max(float64(capacity)-float64(used), 0)
	seconds := remaining / slope
	if seconds > maxFillETA.Seconds() {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}

func bytesPerSecond(samples []usageSample) float64 {
	origin := samples[0].at
	var sumX, sumY, sumXY, sumXX float64
	for _, s := range samples {
		x := s.at.Sub(origin).Seconds()
		sumX += x
		sumY += s.used
		sumXY += x * s.used
		sumXX += x * x
	}
	n := float64(len(samples))
	denominator := n*sumXX - sumX*sumX
	if denominator == 0 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / denominator
}

// forgetBefore drops volumes whose newest sample is older than cutoff, so
// deleted or moved claims do not keep their history forever.
func (g *growthTracker) forgetBefore(cutoff time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for id, samples := range g.samples {
		if len(samples) == 0 || samples[len(samples)-1].at.Before(cutoff) {
			delete(g.samples, id)
		}
	}
}
