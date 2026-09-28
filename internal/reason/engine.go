package reason

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/signal"
)

// DefaultFloor is the minimum score for a hypothesis to be stated as a
// cause. Below it, the cause is reported as unknown.
const DefaultFloor = 0.45

// SignalReader exposes active signals to rules.
type SignalReader interface {
	Active(id knowledge.EntityID) []signal.Signal
	Has(id knowledge.EntityID) bool
}

// Query is what rules may read while explaining one symptom.
type Query struct {
	Model   knowledge.Reader
	Signals SignalReader
	Now     time.Time
}

// Rule proposes hypotheses for a symptom.
type Rule interface {
	Name() string
	Explain(q Query, symptom signal.Signal) []Hypothesis
}

// Engine ranks the hypotheses of every rule.
type Engine struct {
	rules []Rule
	floor float64
}

// NewEngine builds an engine. A non-positive floor takes DefaultFloor.
func NewEngine(floor float64, rules ...Rule) *Engine {
	if floor <= 0 {
		floor = DefaultFloor
	}
	return &Engine{rules: rules, floor: floor}
}

// Explain returns the best supported cause of symptom and the reasoning
// trace. Hypotheses blaming the symptom's own entity are dropped: an
// explanation must say something the symptom does not.
func (e *Engine) Explain(q Query, symptom signal.Signal) Explanation {
	out := Explanation{Symptom: symptom}
	var candidates []Hypothesis
	for _, rule := range e.rules {
		for _, h := range rule.Explain(q, symptom) {
			h.Rule = rule.Name()
			if h.Root == symptom.Entity && h.Change == nil {
				continue
			}
			if h.Score < e.floor {
				out.Rejected = append(out.Rejected, h)
				continue
			}
			candidates = append(candidates, h)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > 0 {
		best := candidates[0]
		out.Cause = &best
		out.Alternatives = candidates[1:]
	}
	return out
}
