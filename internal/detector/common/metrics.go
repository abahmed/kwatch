package common

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

type ResourceThresholds struct {
	CPUPercent    int `mapstructure:"cpuPercent" json:"cpuPercent"`
	MemoryPercent int `mapstructure:"memoryPercent" json:"memoryPercent"`
}

var DefaultThresholds = ResourceThresholds{
	CPUPercent:    80,
	MemoryPercent: 80,
}

func CheckResourceThresholds(pod *corev1.Pod, thresholds ResourceThresholds) (bool, string) {
	for _, container := range pod.Status.ContainerStatuses {
		resources := container.Resources
		if resources == nil {
			continue
		}

		limits := resources.Limits
		if limits == nil {
			continue
		}

		_ = limits.Cpu()
		_ = limits.Memory()
	}
	return false, ""
}

type MetricsCache struct {
	podMetrics  map[string]PodMetrics
	nodeMetrics map[string]NodeMetrics
	pvcUsage    map[string]PVCUsage
	lastUpdated time.Time
	maxAge      time.Duration
}

type PodMetrics struct {
	Name       string
	Namespace  string
	Containers []ContainerMetrics
	Timestamp  time.Time
}

type ContainerMetrics struct {
	Name          string
	CPUUsage      int64
	MemoryUsage   int64
	CPUPercent    float64
	MemoryPercent float64
}

type NodeMetrics struct {
	Name          string
	CPUUsage      int64
	MemoryUsage   int64
	CPUPercent    float64
	MemoryPercent float64
	Timestamp     time.Time
}

type PVCUsage struct {
	Name        string
	Namespace   string
	Capacity    int64
	Used        int64
	Percent     float64
	LastChecked time.Time
	IsAttached  bool
}

func NewMetricsCache(maxAge time.Duration) *MetricsCache {
	return &MetricsCache{
		podMetrics:  make(map[string]PodMetrics),
		nodeMetrics: make(map[string]NodeMetrics),
		pvcUsage:    make(map[string]PVCUsage),
		lastUpdated: time.Now(),
		maxAge:      maxAge,
	}
}

func (m *MetricsCache) IsStale() bool {
	return time.Since(m.lastUpdated) > m.maxAge
}

func (m *MetricsCache) UpdatePodMetrics(key string, metrics PodMetrics) {
	m.podMetrics[key] = metrics
	m.lastUpdated = time.Now()
}

func (m *MetricsCache) GetPodMetrics(key string) (PodMetrics, bool) {
	val, ok := m.podMetrics[key]
	return val, ok
}

func (m *MetricsCache) UpdateNodeMetrics(key string, metrics NodeMetrics) {
	m.nodeMetrics[key] = metrics
	m.lastUpdated = time.Now()
}

func (m *MetricsCache) GetNodeMetrics(key string) (NodeMetrics, bool) {
	val, ok := m.nodeMetrics[key]
	return val, ok
}

func (m *MetricsCache) UpdatePVCUsage(key string, usage PVCUsage) {
	m.pvcUsage[key] = usage
	m.lastUpdated = time.Now()
}

func (m *MetricsCache) GetPVCUsage(key string) (PVCUsage, bool) {
	val, ok := m.pvcUsage[key]
	return val, ok
}

func (m *MetricsCache) GetAllPVCUsage() []PVCUsage {
	result := make([]PVCUsage, 0, len(m.pvcUsage))
	for _, v := range m.pvcUsage {
		result = append(result, v)
	}
	return result
}
