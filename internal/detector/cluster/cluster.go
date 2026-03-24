package cluster

import (
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/detector"
)

// Config holds cluster detection configuration
type Config struct {
	ThresholdPercent float64 // percentage of cluster size
	MinThreshold     int     // minimum pods to trigger
	MaxThreshold     int     // maximum pods to trigger
	Window           time.Duration
}

// Pattern holds cluster-wide pattern info
type Pattern struct {
	Reason       string
	Namespace    string
	AffectedPods map[string]time.Time
	FirstSeen    time.Time
	LastSeen     time.Time
	TotalCount   int
	Threshold    int
}

// Detector detects cluster-wide patterns
type Detector struct {
	config      *Config
	mu          sync.Mutex
	patterns    map[string]*Pattern
	clustersize int
}

func NewDetector(config *Config) *Detector {
	return &Detector{
		config:   config,
		patterns: make(map[string]*Pattern),
	}
}

// SetClusterSize sets the cluster size for threshold calculation
func (d *Detector) SetClusterSize(size int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.clustersize = size
	d.updateThresholds()
}

func (d *Detector) updateThresholds() {
	threshold := int(float64(d.clustersize) * d.config.ThresholdPercent / 100)
	if threshold < d.config.MinThreshold {
		threshold = d.config.MinThreshold
	}
	if threshold > d.config.MaxThreshold {
		threshold = d.config.MaxThreshold
	}

	for _, p := range d.patterns {
		p.Threshold = threshold
	}
}

func (d *Detector) Name() string {
	return "ClusterDetector"
}

func (d *Detector) Detect(input *detector.Input) *detector.Event {
	if !input.HasIssue {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	key := input.IssueType + "/" + input.Reason
	if input.Pod != nil && input.Pod.Namespace != "" {
		key = key + "/" + input.Pod.Namespace
	}

	if pattern, exists := d.patterns[key]; exists {
		pattern.AffectedPods[input.Pod.Name] = time.Now()
		pattern.TotalCount++
		pattern.LastSeen = time.Now()

		// Check if threshold exceeded
		if len(pattern.AffectedPods) >= pattern.Threshold {
			event := &detector.Event{
				Type:      "cluster",
				Name:      "cluster-wide",
				Namespace: input.Pod.Namespace,
				Reason:    pattern.Reason,
				Message:   buildClusterMessage(pattern),
			}
			// Reset after alert
			pattern.AffectedPods = make(map[string]time.Time)
			return event
		}
	} else {
		threshold := int(float64(d.clustersize) * d.config.ThresholdPercent / 100)
		if threshold < d.config.MinThreshold {
			threshold = d.config.MinThreshold
		}
		if threshold > d.config.MaxThreshold {
			threshold = d.config.MaxThreshold
		}

		d.patterns[key] = &Pattern{
			Reason:       input.Reason,
			Namespace:    input.Pod.Namespace,
			AffectedPods: map[string]time.Time{input.Pod.Name: time.Now()},
			FirstSeen:    time.Now(),
			LastSeen:     time.Now(),
			TotalCount:   1,
			Threshold:    threshold,
		}
	}

	return nil
}

func buildClusterMessage(pattern *Pattern) string {
	return "Cluster-wide issue detected"
}
