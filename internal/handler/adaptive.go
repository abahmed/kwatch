package handler

import "time"

// adaptiveSustained adds bounded grace only when a large workload has a small
// partial deficit. It preserves the configured threshold for single-replica
// and materially degraded workloads.
func adaptiveSustained(baseMinutes int, enabled bool, desired, unavailable int32) time.Duration {
	if baseMinutes <= 0 {
		return 0
	}
	minutes := baseMinutes
	if enabled && desired >= 4 && unavailable > 0 && unavailable*4 <= desired {
		bonus := int(desired / 10)
		if bonus < 1 {
			bonus = 1
		}
		if bonus > 5 {
			bonus = 5
		}
		minutes += bonus
	}
	return time.Duration(minutes) * time.Minute
}
