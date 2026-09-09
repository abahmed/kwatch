package handler

import "github.com/abahmed/kwatch/internal/model"

// findings collects a detector's results, dropping the nils that mean
// "nothing found". It exists so a Process function can hand the reconciler
// every detector's answer in one slice without each caller writing the same
// nil checks.
func findings(observations ...*model.Observation) []*model.Observation {
	var out []*model.Observation
	for _, obs := range observations {
		if obs != nil {
			out = append(out, obs)
		}
	}
	return out
}
