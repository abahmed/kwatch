package reason

import (
	"sort"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/signal"
)

// Point is one piece of evidence for or against a hypothesis.
type Point struct {
	Text     string
	Weight   float64
	Supports bool
}

// Hypothesis proposes one root cause for a symptom.
type Hypothesis struct {
	Rule string
	// Root is the entity blamed for the symptom.
	Root knowledge.EntityID
	// RootSignals are the root's own active signals, if any.
	RootSignals []signal.Signal
	// Change is the change blamed, when the cause is a modification.
	Change *knowledge.Change
	// Chain is the path from the root to the symptom's entity.
	Chain []knowledge.EntityID
	// Summary states the cause in one sentence.
	Summary string
	Points  []Point
	Score   float64
}

// Explanation is the engine's conclusion for one symptom.
type Explanation struct {
	Symptom signal.Signal
	// Cause is nil when no hypothesis reaches the confidence floor.
	Cause        *Hypothesis
	Alternatives []Hypothesis
	Rejected     []Hypothesis
}

// Confidence levels shown to readers.
const (
	High   = 0.75
	Likely = 0.5
)

// scorer accumulates evidence into a bounded score. Contradicting evidence
// subtracts; the result is clamped to [0, 1].
type scorer struct {
	score  float64
	points []Point
}

func newScorer(prior float64) *scorer { return &scorer{score: prior} }

func (s *scorer) support(weight float64, text string) {
	s.score += weight
	s.points = append(s.points, Point{Text: text, Weight: weight,
		Supports: true})
}

func (s *scorer) contradict(weight float64, text string) {
	s.score -= weight
	s.points = append(s.points, Point{Text: text, Weight: weight})
}

func (s *scorer) result() (float64, []Point) {
	sort.SliceStable(s.points, func(i, j int) bool {
		return s.points[i].Weight > s.points[j].Weight
	})
	return min(max(s.score, 0), 1), s.points
}
