package pvc

import "github.com/abahmed/kwatch/internal/model"

// cloneSamples prevents a persistence implementation from sharing its
// mutable map with the monitor's in-memory state.
func cloneSamples(
	samples map[string]model.PVCSample,
) map[string]model.PVCSample {
	if samples == nil {
		return nil
	}
	result := make(map[string]model.PVCSample, len(samples))
	for key, sample := range samples {
		result[key] = sample
	}
	return result
}

func clonePVMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
