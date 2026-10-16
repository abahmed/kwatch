package pipeline

import "github.com/abahmed/kwatch/internal/feature"

// entitled reports whether the installation may use id.
func (d Dependencies) entitled(id feature.ID) bool {
	return feature.OrCommunity(d.Entitlements).Has(id)
}

// baselineStart reports that incidents are neither restored nor saved:
// the failures that exist at start are the baseline, and only new ones
// are announced.
func (d Dependencies) baselineStart() bool {
	return !d.entitled(feature.DiskState)
}
