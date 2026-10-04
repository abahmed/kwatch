package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// state is what one side of a row is matched against: an entity, the
// modes it shows and their health.
type state struct {
	id    inventory.EntityID
	modes []modeHealth
	// text is the effect's error text, for signals.
	text string
}

// modeHealth is one mode an entity shows with the health it implies.
type modeHealth struct {
	mode   detection.Mode
	health detection.Health
	// pseudo marks a mode that is not a finding (a change, an
	// absence). A side matches it only when it names it.
	pseudo bool
}

// anyModeMatches reports whether mode is one of wants or a finer mode
// below one of them.
func anyModeMatches(wants []detection.Mode, mode detection.Mode) bool {
	for _, want := range wants {
		if mode.Within(want) {
			return true
		}
	}
	return false
}

// kindMatches reports whether the side selects the entity's kind. Pod
// sides also select containers, which fail on behalf of their pod.
func (s Side) kindMatches(id inventory.EntityID) bool {
	if s.Group != AnyGroup && s.Group != id.Group {
		return false
	}
	if s.Custom && id.Group == "" {
		return false
	}
	if s.Kind == AnyKind || s.Kind == id.Kind {
		return true
	}
	return s.Kind == kube.KindPod && id.Kind == kube.KindContainer
}

// healthMatches reports whether health is one the side accepts.
func (s Side) healthMatches(health detection.Health) bool {
	if len(s.Health) == 0 {
		return health == detection.Failing || health == detection.Degraded
	}
	for _, want := range s.Health {
		if want == health {
			return true
		}
	}
	return false
}

// matches reports whether the side selects the state, and the first
// mode that made it match.
func (s Side) matches(st state) (modeHealth, bool) {
	if !s.kindMatches(st.id) || !hasSignal(s.Signal, st.text) {
		return modeHealth{}, false
	}
	for _, mh := range st.modes {
		if !s.healthMatches(mh.health) ||
			anyModeMatches(s.NotModes, mh.mode) {
			continue
		}
		if (len(s.Modes) == 0 && !mh.pseudo) ||
			anyModeMatches(s.Modes, mh.mode) {
			return mh, true
		}
	}
	return modeHealth{}, false
}

// rowMatch is the row that links one cause to one effect.
type rowMatch struct {
	row        Row
	causeMode  detection.Mode
	effectMode detection.Mode
	// inferred marks a cause that matched on a pseudo mode: a change,
	// an absence, or a state read from the effect's own text.
	inferred bool
}

// minWorkloads is the fewest distinct workloads the match needs.
func (m rowMatch) minWorkloads() int {
	if m.inferred && m.row.InferredMinWorkloads > m.row.MinWorkloads {
		return m.row.InferredMinWorkloads
	}
	return m.row.MinWorkloads
}

// bestRow returns the matching row with the highest prior. Rows are
// scanned in table order, so on equal priors the first row wins.
func bestRow(
	table []Row, cause state, link LinkType, effect state,
) (rowMatch, bool) {
	var best rowMatch
	found := false
	for _, row := range table {
		if row.Link != link || (found && row.Prior <= best.row.Prior) {
			continue
		}
		causeMode, ok := row.Cause.matches(cause)
		if !ok {
			continue
		}
		effectMode, ok := row.Effect.matches(effect)
		if !ok {
			continue
		}
		best = rowMatch{row: row, causeMode: causeMode.mode,
			effectMode: effectMode.mode, inferred: causeMode.pseudo}
		found = true
	}
	return best, found
}

// findingModes returns the modes of findings that are not healthy.
func findingModes(findings []detection.Finding) []modeHealth {
	var out []modeHealth
	for _, f := range findings {
		if f.Health == detection.Healthy || f.Advisory {
			continue
		}
		out = append(out, modeHealth{mode: f.Mode, health: f.Health})
	}
	return out
}

// unhealthy reports whether a finding shows the entity failing or
// degraded.
func unhealthy(f detection.Finding) bool {
	// A risk is advice about configuration, not a failure to explain
	// nor a state that explains anything.
	return !f.Advisory &&
		(f.Health == detection.Failing || f.Health == detection.Degraded)
}
