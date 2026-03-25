package aggregator

import (
	"time"

	"github.com/abahmed/kwatch/internal/detector"
	"github.com/abahmed/kwatch/internal/detector/store"
)

type ClusterAggregator struct {
	clusterStore *store.ClusterStore
}

func NewClusterAggregator(clusterStore *store.ClusterStore) *ClusterAggregator {
	return &ClusterAggregator{
		clusterStore: clusterStore,
	}
}

func (c *ClusterAggregator) Name() string {
	return "ClusterAggregator"
}

func (c *ClusterAggregator) Record(input *detector.Input) {
	c.clusterStore.Record(input)
}

func (c *ClusterAggregator) DetectClusterIssue(input *detector.Input) *detector.Event {
	return c.clusterStore.DetectClusterIssue(input)
}

func (c *ClusterAggregator) IsClusterIssue(input *detector.Input) bool {
	return c.clusterStore.IsClusterIssue(input)
}

func (c *ClusterAggregator) GetAllPatterns() []*store.ClusterPattern {
	return c.clusterStore.GetAllPatterns()
}

func (c *ClusterAggregator) GetStats() ClusterAggregatorStats {
	stats := c.clusterStore.GetStats()
	return ClusterAggregatorStats{
		TotalPatterns: stats.TotalPatterns,
		TotalPods:     stats.TotalPods,
		Threshold:     stats.Threshold,
		Window:        stats.Window,
	}
}

type ClusterAggregatorStats struct {
	TotalPatterns int
	TotalPods     int
	Threshold     int
	Window        time.Duration
}
