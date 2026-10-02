package detectors

import "github.com/abahmed/kwatch/internal/detection"

// Default returns the production detector set, in the order the
// registry evaluates them, with default thresholds. The application and
// every test harness build their registry from it, so a detector added
// here is detected everywhere.
func Default() []detection.Detector {
	return []detection.Detector{
		Container{},
		Generic{},
		NewPod(PodThresholds{}),
		NewNode(0),
		NewWorkload(0),
		Job{},
		HPA{},
		Claim{},
		Volume{},
		Service{},
		Certificate{},
		Missing{},
		Event{},
		Budget{},
		Quota{},
		Attachment{},
		Webhook{},
		NodeUsage{},
		VolumeUsage{},
		Custom{},
		ClusterService{},
		Ingress{},
		EgressPolicy{},
		Schedule{},
		Namespace{},
		ContainerResources{},
		PodStorage{},
		NodeHealth{},
		VersionSkew{},
		ActiveProbe{},
	}
}
