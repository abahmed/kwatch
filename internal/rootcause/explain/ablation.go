package explain

import "slices"

// Ablation switches parts of the scoring off. Only the scorecard's
// mutation check sets it: it removes a scorer or every veto from the
// engine and asserts that some alert-quality gate then fails, which
// proves the gates can see a root-cause regression. The zero value
// changes nothing, and production never sets it.
type Ablation struct {
	// Scorers lists scorers to skip, by name ("specificity",
	// "image-elsewhere" and so on).
	Scorers []string
	// Vetoes ignores every veto a scorer raises: a vetoed candidate
	// stays in the running.
	Vetoes bool
}

// skips reports whether the scorer called name is switched off.
func (a Ablation) skips(name string) bool {
	return slices.Contains(a.Scorers, name)
}

// without returns o with its veto removed when vetoes are ablated.
func (a Ablation) without(o outcome) outcome {
	if a.Vetoes {
		o.veto = ""
	}
	return o
}

// CounterfactualScorers names the counterfactual checks, which compare
// a cause with healthy twins. The mutation check switches them off
// together.
func CounterfactualScorers() []string {
	names := make([]string, 0, len(counterfactualScorers))
	for _, s := range counterfactualScorers {
		names = append(names, s.name)
	}
	return names
}
