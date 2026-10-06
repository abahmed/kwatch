package explain

import (
	"sort"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Contribution is one scorer's contribution to a cause. Weight is
// signed: positive supports the cause, negative contradicts it. Code and
// the numbers say what the scorer found; Text says it in words for the
// trace. Writers read the code, never the text.
type Contribution struct {
	Scorer string
	Code   rootcause.ProofCode
	// Count and Total are the numbers Code counts (see rootcause.Proof).
	Count, Total int
	// Fields are the field paths of a rootcause.ProofChanged change.
	Fields []string
	// Edits are the values a rootcause.ProofNewRevisionFails revision
	// changed, likeliest culprit first. Count is the revision number.
	Edits  []inventory.FieldChange
	Weight float64
	Text   string
}

// outcome is what a scorer found. A zero outcome says nothing. A veto
// rejects the candidate outright, with the veto as the reason.
type outcome struct {
	weight float64
	code   rootcause.ProofCode
	// count, total and fields are the data of the code.
	count, total int
	fields       []string
	edits        []inventory.FieldChange
	text         string
	veto         string
}

// said reports whether the scorer found anything.
func (o outcome) said() bool { return o.text != "" || o.veto != "" }

// scorer reads the view and one candidate and returns its evidence.
type scorer struct {
	name  string
	score func(v *view, c *candidate) outcome
}

// scorers run in this order; the order only affects the trace.
var scorers = []scorer{
	{"temporal", scoreTemporal},
	{"coverage", scoreCoverage},
	{"exclusivity", scoreExclusivity},
	{"specificity", scoreSpecificity},
	{"change", scoreChange},
	{"outcome", scoreOutcome},
	{"revision", scoreRevision},
	{"shared", scoreShared},
	{"baseline", scoreBaseline},
	{"quality", scoreQuality},
}

func init() {
	// The counterfactual checks come last: they compare with healthy
	// twins and only break ties the other evidence leaves open.
	scorers = append(scorers, counterfactualScorers...)
}

// scored is a candidate with its confidence and evidence.
type scored struct {
	c *candidate
	// raw is the unclamped sum; it ranks candidates whose confidence
	// is capped at 1 alike.
	raw           float64
	confidence    float64
	contributions []Contribution
	veto          string
}

// score runs every scorer on a candidate. The prior is the mean of the
// rows that link it to its effects, lowered for long chains.
func (v *view) score(c *candidate) scored {
	out := scored{c: c}
	total := priorOf(c)
	for _, s := range scorers {
		o := s.score(v, c)
		if !o.said() {
			continue
		}
		if o.veto != "" && out.veto == "" {
			out.veto = o.veto
		}
		total += o.weight
		out.contributions = append(out.contributions, Contribution{
			Scorer: s.name, Code: o.code, Count: o.count, Total: o.total,
			Fields: o.fields, Edits: o.edits, Weight: o.weight,
			Text: firstNonEmpty(o.text, o.veto),
		})
	}
	sort.SliceStable(out.contributions, func(i, j int) bool {
		return abs(out.contributions[i].Weight) >
			abs(out.contributions[j].Weight)
	})
	if sharedFactorOnly(c) {
		total = min(total, SharedFactorMaxConfidence)
	}
	out.raw = total
	out.confidence = min(max(total, 0), 1)
	return out
}

// sharedFactorOnly reports a candidate covering its effects only as
// what they have in common (ModeSharedFactor), with no finding or
// change of its own to blame.
func sharedFactorOnly(c *candidate) bool {
	shared := false
	for _, how := range c.covers {
		if how.derived || how.match.row.Name == selfRow.Name {
			continue
		}
		if how.match.causeMode != ModeSharedFactor {
			return false
		}
		shared = true
	}
	return shared
}

// priorOf averages the priors of the rows that cover other entities,
// and the candidate's own failure when a self row (its own change) says
// why. A candidate covering only itself keeps the self prior.
func priorOf(c *candidate) float64 {
	sum, n := 0.0, 0
	for _, effect := range c.direct() {
		how := c.covers[effect]
		if effect == c.id && how.match.row.Name == selfRow.Name {
			continue
		}
		sum += how.match.row.Prior * decay(len(how.chain)-1, effect)
		n++
	}
	if n == 0 {
		return c.selfPrior()
	}
	return sum / float64(n)
}

// decay lowers a prior by HopDecay for every link past the first. The
// hop from a pod to its container does not count.
func decay(links int, effect inventory.EntityID) float64 {
	if effect.Kind == "container" {
		links--
	}
	factor := 1.0
	for i := 1; i < links; i++ {
		factor *= HopDecay
	}
	return factor
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// earliestSince is the earliest start of the unhealthy findings of
// ids. ok is false when none has a start time.
func (v *view) earliestSince(ids []inventory.EntityID) (t timeValue) {
	for _, id := range ids {
		for _, f := range v.s.Findings[id] {
			if unhealthy(f) && !f.Since.IsZero() {
				t = t.earlier(f.Since)
			}
		}
	}
	return t
}

// hasUnknown reports whether id has a finding of Unknown health.
func (v *view) hasUnknown(id inventory.EntityID) bool {
	for _, f := range v.s.Findings[id] {
		if f.Health == detection.Unknown {
			return true
		}
	}
	return false
}
