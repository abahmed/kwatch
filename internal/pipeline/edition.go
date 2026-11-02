package pipeline

import "github.com/abahmed/kwatch/internal/feature"

// entitled reports whether the installation may use id.
func (d Dependencies) entitled(id feature.ID) bool {
	return feature.OrCommunity(d.Entitlements).Has(id)
}

// baselineStart reports a Community start: the failures that exist at
// start are the baseline, and only new ones are announced. Only the
// incidents that were announced and are still open are saved and
// restored, so their resolves are sent after a restart (see
// persistence.openOnly); fingerprints, history and the startup marker
// are not kept.
func (d Dependencies) baselineStart() bool {
	return !d.entitled(feature.DiskState)
}

// summarisesStartup reports that the failures found at a cold start are
// listed in one startup summary and the incidents restored after a
// restart in one restored listing. Without it every failure that exists
// at start is the baseline (Community).
func (d Dependencies) summarisesStartup() bool {
	return !d.baselineStart() && d.entitled(feature.StartupSummary)
}
