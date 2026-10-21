package scenarios

import (
	"slices"
	"strings"

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
	// blamed lists must-not-blame entities that a stated cause, its
	// rival or a message named as the cause.
	blamed []string
	// firstWrong lists the roots the first message stated that the label
	// does not allow; announcedWrong lists them over every message (see
	// announced_test.go). An incident that states no cause is never
	// wrong: it says the cause is unknown.
	firstWrong, announcedWrong []string
	// tier is the first incident's delivery tier, empty when none.
	tier string
	// pages is how many paging alerts the replay opened.
	pages int
	// falseRecoveries counts resolves that said "recovered" while a
	// failure the incident explained still failed elsewhere.
	falseRecoveries int
	// leftOpen counts announced incidents still open with no member.
	leftOpen int
}

// overPages reports more paging alerts than the label allows.
func (v verdict) overPages() bool {
	return v.pages > pageLimit(v.expect)
}

// overBudget reports more messages than the label allows.
func (v verdict) overBudget() bool { return v.messages > v.expect.MaxMessages }

// loud reports a quiet scenario that produced a message.
func (v verdict) loud() bool { return v.expect.Quiet && v.messages > 0 }

// tierMatches reports whether the first incident has the expected tier.
func (v verdict) tierMatches() bool {
	return v.expect.Quiet || v.tier == v.expect.Tier
}

// problems lists every way a scenario missed its label, in the words of
// the scenario table. A scenario that passes has none.
func (v verdict) problems() []string {
	var misses []string
	add := func(miss bool, text string) {
		if miss {
			misses = append(misses, text)
		}
	}
	add(slices.ContainsFunc(v.cases, func(c scorecard.Case) bool {
		return !c.Correct
	}), "wrong root")
	add(slices.ContainsFunc(v.cases, scorecard.Case.WrongStated),
		"wrong root stated")
	if len(v.blamed) > 0 {
		misses = append(misses, "blamed "+strings.Join(v.blamed, ", "))
	}
	add(v.wrongFirstRoot(), "wrong root first: "+
		strings.Join(v.firstWrong, ", "))
	add(v.loud(), "not quiet")
	add(v.overBudget(), "over budget")
	add(!v.tierMatches(), "tier")
	add(v.overPages(), "over pages")
	add(v.leftOpen > 0, "left open")
	add(v.falseRecoveries > 0, "false recovery")
	return misses
}

// judge compares a replay with its label. The log is only read for who
// owns what, so a message about a pod can count as one about its
// Deployment.
func judge(e expectation, log replay.Log, result replay.Result) verdict {
	v := verdict{
		expect: e, messages: len(result.Messages),
		incidents: withoutUnrelated(e,
			heardFirst(reportedIncidents(result))),
	}
	if len(v.incidents) > 0 {
		v.tier = v.incidents[0].Tier.String()
	}
	v.cases = rootCases(e, v.incidents)
	v.blamed = blamed(e, result)
	v.firstWrong, v.announcedWrong = wrongRootsAnnounced(
		e, ownershipOf(log), result)
	v.pages = pagesRaised(result)
	v.falseRecoveries = falseRecoveries(result)
	v.leftOpen = leftOpen(result)
	return v
}

// withoutUnrelated drops the incidents the label names as real but
// about other entities, so the first root is judged against the first
// incident about the scenario's own subject.
func withoutUnrelated(e expectation, incidents []reported) []reported {
	if len(e.Unrelated) == 0 {
		return incidents
	}
	out := make([]reported, 0, len(incidents))
	for _, p := range incidents {
		if !slices.Contains(e.Unrelated, p.Root) {
			out = append(out, p)
		}
	}
	return out
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

// rootCases judges each expected root. A scenario with one root judges
// it against the first reported incident. Several roots are independent
// incidents that may be heard in any order, so each is matched to an
// incident of its own.
func rootCases(e expectation, incidents []reported) []scorecard.Case {
	var cases []scorecard.Case
	used := map[int]bool{}
	for i, want := range e.roots() {
		c := scorecard.Case{
			Name: e.Name, Expected: want, Actual: "no matching incident",
			ExpectsRoot: want != rootUnknown, Ambiguous: e.Ambiguous,
		}
		var match int
		if i == 0 && len(e.roots()) == 1 && len(incidents) > 0 {
			match = firstIncident(incidents, want)
		} else {
			match = findRoot(incidents, want, used)
		}
		if match >= 0 {
			used[match] = true
			got := incidents[match]
			c.Actual, c.Confidence = actualRoot(got), got.confidence()
			c.Correct = rootMatches(want, got)
			c.Hedged = got.Cause != nil && got.Cause.Rival != nil
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
