package cluster

import (
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/detector"
)

type Config struct {
	ThresholdPercent float64
	MinThreshold     int
	MaxThreshold     int
	Window           time.Duration
}

type Pattern struct {
	Reason       string
	Namespace    string
	AffectedPods map[string]time.Time
	FirstSeen    time.Time
	LastSeen     time.Time
	TotalCount   int
	Threshold    int
}

type Detector struct {
	config      *Config
	mu          sync.Mutex
	patterns    map[string]*Pattern
	clustersize int
}

func NewDetector(cfg *Config) *Detector {
	if cfg == nil {
		cfg = &Config{
			ThresholdPercent: 5.0,
			MinThreshold:     5,
			MaxThreshold:     20,
			Window:           15 * time.Minute,
		}
	}
	return &Detector{
		config:   cfg,
		patterns: make(map[string]*Pattern),
	}
}

func (d *Detector) Name() string {
	return "ClusterDetector"
}

func (d *Detector) Detect(input *detector.Input) bool {
	if input == nil || input.Pod == nil || !input.HasIssue {
		return false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	key := input.IssueType + "/" + input.Reason
	if input.Pod.Namespace != "" {
		key = key + "/" + input.Pod.Namespace
	}

	pattern, exists := d.patterns[key]
	if !exists {
		threshold := d.calculateThreshold()
		pattern = &Pattern{
			Reason:       input.Reason,
			Namespace:    input.Pod.Namespace,
			AffectedPods: make(map[string]time.Time),
			FirstSeen:    time.Now(),
			LastSeen:     time.Now(),
			TotalCount:   0,
			Threshold:    threshold,
		}
		d.patterns[key] = pattern
	}

	pattern.AffectedPods[input.Pod.Name] = time.Now()
	pattern.TotalCount++
	pattern.LastSeen = time.Now()

	return len(pattern.AffectedPods) >= pattern.Threshold
}

func (d *Detector) calculateThreshold() int {
	threshold := int(float64(d.clustersize) * d.config.ThresholdPercent / 100)
	if threshold < d.config.MinThreshold {
		threshold = d.config.MinThreshold
	}
	if threshold > d.config.MaxThreshold {
		threshold = d.config.MaxThreshold
	}
	return threshold
}

func (d *Detector) SetClusterSize(size int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.clustersize = size
}

func (d *Detector) GetPatterns() map[string]*Pattern {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make(map[string]*Pattern)
	for k, v := range d.patterns {
		result[k] = v
	}
	return result
}

func (d *Detector) Cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	cutoff := time.Now().Add(-d.config.Window)
	for key, pattern := range d.patterns {
		if pattern.LastSeen.Before(cutoff) {
			delete(d.patterns, key)
		}
	}
}

func BuildClusterMessage(pattern *Pattern) string {
	return "Cluster-wide issue detected: " + pattern.Reason +
		" affecting " + formatCount(len(pattern.AffectedPods)) + " pods"
}

func formatCount(n int) string {
	if n == 1 {
		return "1 pod"
	}
	return string(rune(n+'0')) + " pods"
}
