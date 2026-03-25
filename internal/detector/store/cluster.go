package store

import (
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/detector"
)

const ClusterKeyPrefix = "cluster:"

type ClusterStore struct {
	store     *Store
	patterns  map[string]*ClusterPattern
	threshold int
	window    time.Duration
	mu        sync.RWMutex
}

type ClusterPattern struct {
	Reason     string
	Namespaces map[string]int
	Pods       map[string]bool
	Count      int
	FirstSeen  time.Time
	LastSeen   time.Time
}

func NewClusterStore(store *Store, threshold int, window time.Duration) *ClusterStore {
	c := &ClusterStore{
		store:     store,
		patterns:  make(map[string]*ClusterPattern),
		threshold: threshold,
		window:    window,
	}
	if threshold == 0 {
		c.threshold = 5
	}
	if window == 0 {
		c.window = 15 * time.Minute
	}
	c.load()
	return c
}

func (c *ClusterStore) Name() string {
	return "ClusterStore"
}

func (c *ClusterStore) Record(input *detector.Input) {
	if input.Pod == nil || input.Reason == "" {
		return
	}

	key := input.Reason
	c.mu.Lock()
	defer c.mu.Unlock()

	pattern, exists := c.patterns[key]
	if !exists {
		c.patterns[key] = &ClusterPattern{
			Reason:     input.Reason,
			Namespaces: make(map[string]int),
			Pods:       make(map[string]bool),
			FirstSeen:  time.Now(),
			LastSeen:   time.Now(),
		}
		pattern = c.patterns[key]
	}

	podKey := input.Pod.Namespace + "/" + input.Pod.Name
	if !pattern.Pods[podKey] {
		pattern.Pods[podKey] = true
		pattern.Namespaces[input.Pod.Namespace]++
		pattern.Count++
	}
	pattern.LastSeen = time.Now()

	c.save()
}

func (c *ClusterStore) IsClusterIssue(input *detector.Input) bool {
	if input.Pod == nil || input.Reason == "" {
		return false
	}

	key := input.Reason
	c.mu.RLock()
	defer c.mu.RUnlock()

	pattern, exists := c.patterns[key]
	if !exists {
		return false
	}

	return pattern.Count >= c.threshold
}

func (c *ClusterStore) GetPattern(reason string) *ClusterPattern {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.patterns[reason]
}

func (c *ClusterStore) GetAllPatterns() []*ClusterPattern {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*ClusterPattern, 0, len(c.patterns))
	for _, p := range c.patterns {
		result = append(result, p)
	}
	return result
}

func (c *ClusterStore) load() {
	c.mu.Lock()
	defer c.mu.Unlock()

	var patterns map[string]*ClusterPattern
	err := c.store.Read(ClusterKeyPrefix+"patterns", &patterns)
	if err != nil || patterns == nil {
		c.patterns = make(map[string]*ClusterPattern)
		return
	}
	c.patterns = patterns
}

func (c *ClusterStore) save() {
	if c.store == nil {
		return
	}
	_ = c.store.Write(ClusterKeyPrefix+"patterns", c.patterns)
}

func (c *ClusterStore) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	cutoff := time.Now().Add(-c.window)
	for key, pattern := range c.patterns {
		if pattern.LastSeen.Before(cutoff) {
			delete(c.patterns, key)
		}
	}
	c.save()
}

func (c *ClusterStore) GetStats() ClusterStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalPods := 0
	for _, p := range c.patterns {
		totalPods += p.Count
	}

	return ClusterStats{
		TotalPatterns: len(c.patterns),
		TotalPods:     totalPods,
		Threshold:     c.threshold,
		Window:        c.window,
	}
}

type ClusterStats struct {
	TotalPatterns int
	TotalPods     int
	Threshold     int
	Window        time.Duration
}

func (c *ClusterStore) DetectClusterIssue(input *detector.Input) *detector.Event {
	if !c.IsClusterIssue(input) {
		return nil
	}

	pattern := c.GetPattern(input.Reason)
	if pattern == nil {
		return nil
	}

	return &detector.Event{
		Type:      "cluster",
		Name:      "ClusterIssue",
		Reason:    input.Reason,
		Message:   formatClusterMessage(pattern),
		Namespace: "cluster-wide",
	}
}

func formatClusterMessage(pattern *ClusterPattern) string {
	return "Cluster-wide issue detected: " + pattern.Reason +
		" affecting " + formatPodCount(pattern.Count) + " pods across " +
		formatNamespaceCount(pattern.Namespaces)
}

func formatPodCount(count int) string {
	if count == 1 {
		return "1 pod"
	}
	return string(rune(count)) + " pods"
}

func formatNamespaceCount(namespaces map[string]int) string {
	if len(namespaces) == 1 {
		for ns := range namespaces {
			return "namespace " + ns
		}
	}
	return "multiple namespaces"
}
