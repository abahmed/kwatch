package scenarios

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/scorecard"
)

// verdict is how one scenario replay compares with its label.
type verdict struct {
	expect    expectation
	incidents []reported
	messages  int
	// cases are the labelled root cases, one per expected root.
	cases []scorecard.Case
	// blamed lists must-not-blame entities that a stated cause named.
	blamed []string
	// tier is the first incident's delivery tier, empty when none.
	tier string
}

// overBudget reports more messages than the label allows.
func (v verdict) overBudget() bool { return v.messages > v.expect.MaxMessages }

// loud reports a quiet scenario that produced a message.
func (v verdict) loud() bool { return v.expect.Quiet && v.messages > 0 }

// tierMatches reports whether the first incident has the expected tier.
func (v verdict) tierMatches() bool {
	return v.expect.Quiet || v.tier == v.expect.Tier
}

func judge(e expectation, result replay.Result) verdict {
	v := verdict{
		expect: e, incidents: heardFirst(reportedIncidents(result)),
		messages: len(result.Messages),
	}
	if len(v.incidents) > 0 {
		v.tier = v.incidents[0].Tier.String()
	}
	v.cases = rootCases(e, v.incidents)
	v.blamed = blamed(e, result)
	return v
}

// heardFirst orders incidents as people hear them: notify and page
// incidents interrupt at once, while digest and silent ones wait for the
// digest. Within each group the announcement order is kept.
func heardFirst(incidents []reported) []reported {
	out := make([]reported, 0, len(incidents))
	for _, interrupting := range []bool{true, false} {
		for _, p := range incidents {
			if (p.Tier >= incident.Notify) == interrupting {
				out = append(out, p)
			}
		}
	}
	return out
}

// rootCases judges each expected root. The first root is judged against
// the first reported incident; every further root needs an incident of
// its own among the rest.
func rootCases(e expectation, incidents []reported) []scorecard.Case {
	var cases []scorecard.Case
	used := map[int]bool{}
	for i, want := range e.roots() {
		c := scorecard.Case{
			Name: e.Name, Expected: want, Actual: "no matching incident",
		}
		match := -1
		if i == 0 && len(incidents) > 0 {
			match = firstIncident(incidents, want)
		} else if i > 0 {
			match = findRoot(incidents, want, used)
		}
		if match >= 0 {
			used[match] = true
			got := incidents[match]
			c.Actual, c.Confidence = actualRoot(got), got.confidence()
			c.Correct = rootMatches(want, got)
		}
		cases = append(cases, c)
	}
	return cases
}

// firstIncident picks the incident the first root is judged against: the
// first one reported. Incidents reported together (one startup summary)
// have no order, so any of them with the expected root counts as first.
func firstIncident(incidents []reported, want string) int {
	for i, got := range incidents {
		if !got.First.Equal(incidents[0].First) {
			break
		}
		if rootMatches(want, got) {
			return i
		}
	}
	return 0
}

func findRoot(incidents []reported, want string, used map[int]bool) int {
	for i, got := range incidents {
		if !used[i] && rootMatches(want, got) {
			return i
		}
	}
	return -1
}

// rootMatches reports whether an incident has the expected root. An
// expected "unknown" matches an incident that states no cause.
func rootMatches(want string, got reported) bool {
	if want == rootUnknown {
		return got.Cause == nil
	}
	return got.Root == want
}

// actualRoot renders what the engine concluded: the root, and whether a
// cause was stated for it.
func actualRoot(got reported) string {
	if got.Cause == nil {
		return got.Root + " (no cause)"
	}
	return got.Root
}

// blamed lists the must-not-blame entities any stated cause named, in any
// decision or in the final state.
func blamed(e expectation, result replay.Result) []string {
	forbidden := map[string]bool{}
	for _, id := range e.MustNotBlame {
		forbidden[id] = true
	}
	hit := map[string]bool{}
	var out []string
	check := func(id string) {
		if forbidden[id] && !hit[id] {
			hit[id] = true
			out = append(out, id)
		}
	}
	for _, d := range result.Decisions {
		if cause := d.Incident.Cause; cause != nil {
			check(d.Incident.Root.String())
			check(cause.Root.String())
		}
	}
	for _, p := range result.Incidents {
		if p.Cause != nil && !p.Announced.IsZero() {
			check(p.Root.String())
			check(p.Cause.Root.String())
		}
	}
	return out
}
