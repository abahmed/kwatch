package incident

import (
	"slices"
	"sort"

	"github.com/abahmed/kwatch/internal/detection"
)

// An incident's failures show up one at a time: a crash loop first, the
// workload's unavailability a minute later. Its Mode follows whichever
// finding is the root's own at the moment, so it alone cannot say
// whether a later failure of the same root is the same failure. The set
// of every mode the incident's members had does.

// noteModes adds the modes of p's failing members to the set.
func (p *Incident) noteModes() {
	for _, s := range p.Members {
		if s.Mode == "" || isRisk(s) {
			continue
		}
		if p.modes == nil {
			p.modes = map[detection.Mode]struct{}{}
		}
		p.modes[s.Mode] = struct{}{}
	}
}

// modeList is the sorted set of modes the incident had, its Mode
// included, for the persisted record and its occurrences.
func (p *Incident) modeList() []detection.Mode {
	out := make([]detection.Mode, 0, len(p.modes)+1)
	for mode := range p.modes {
		out = append(out, mode)
	}
	if p.Mode != "" {
		if _, have := p.modes[p.Mode]; !have {
			out = append(out, p.Mode)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// hasMode reports whether mode is the incident's mode or one of the
// modes its members had. An empty mode matches anything.
func (p *Incident) hasMode(mode detection.Mode) bool {
	if mode == "" || p.Mode == "" || p.Mode == mode {
		return true
	}
	_, ok := p.modes[mode]
	return ok
}

// sharesMode reports whether the occurrence failed in a mode that p
// has, or the other way round.
func (o Occurrence) sharesMode(p Incident) bool {
	if o.Mode == p.Mode || slices.Contains(o.Modes, p.Mode) {
		return true
	}
	for mode := range p.modes {
		if mode == o.Mode || slices.Contains(o.Modes, mode) {
			return true
		}
	}
	return false
}
