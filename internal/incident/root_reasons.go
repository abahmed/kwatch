package incident

// RootReasons are the reasons of the root's own findings seen while the
// incident lasted, sorted. A resolved incident has no members left, so
// this is what a resolve message can still say ended.
func (p Incident) RootReasons() []string { return p.rootReasonList() }
