package incident

// LiveTier is the tier the failing members of the incident call for now.
// Unlike Tier it is not kept at the loudest tier the incident reached, and
// it knows nothing of severity overrides: it says what the members are
// worth, so a writer can tell an incident whose findings are still
// notify or page news from one that is only still open.
func (p Incident) LiveTier() Tier { return tier(&p) }
