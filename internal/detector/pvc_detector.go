package detector

// PVCUsageDetector detects PVC usage threshold breaches
type PVCUsageDetector struct {
	threshold float64
}

func NewPVCUsageDetector(threshold float64) *PVCUsageDetector {
	return &PVCUsageDetector{
		threshold: threshold,
	}
}

func (d *PVCUsageDetector) Name() string {
	return "PVCUsageDetector"
}

func (d *PVCUsageDetector) Detect(input *Input) bool {
	// PVC detection is handled through separate polling mechanism
	// This is a placeholder for future implementation
	return false
}
