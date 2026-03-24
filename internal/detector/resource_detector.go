package detector

// ResourceConfig holds resource threshold configuration
type ResourceConfig struct {
	CPUThreshold    float64
	MemoryThreshold float64
}

// ResourceDetector detects CPU/Memory threshold breaches
type ResourceDetector struct {
	config *ResourceConfig
}

func NewResourceDetector(config *ResourceConfig) *ResourceDetector {
	return &ResourceDetector{
		config: config,
	}
}

func (d *ResourceDetector) Name() string {
	return "ResourceDetector"
}

func (d *ResourceDetector) Detect(input *Input) bool {
	// Resource detection is handled separately through metrics polling
	// This is a placeholder for future implementation
	return false
}
