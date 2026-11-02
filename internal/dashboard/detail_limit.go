package dashboard

import (
	"encoding/json"
	"strings"
)

// detailBudget is the largest marshalled detail writeJSON accepts: the
// response limit less the trailing newline.
const detailBudget = maxResponseBytes - 1

// shrinkStep makes the detail smaller once, recording what it left out
// in t, and reports whether it changed anything. floor is how much of
// the part the step keeps at least.
type shrinkStep func(d *IncidentDetail, t *DetailTruncation, floor int) bool

// shrinkStage is one step and its floor.
type shrinkStage struct {
	step  shrinkStep
	floor int
}

// shrinkStages is the fixed order a detail too large for the response
// is cut in: the oldest of each list first, keeping a useful amount,
// then long strings, then everything that remains optional, then the
// strings again, shorter. Every list of the detail has a stage, so a
// detail fits whatever it holds. The fixed order keeps the result the
// same for the same incident.
var shrinkStages = []shrinkStage{
	{trimTimeline, 20},
	{trimLogLines, 20},
	{trimEvidenceLines, 10},
	{shortenTexts, 500},
	{trimTimeline, 0},
	{trimLogLines, 0},
	{trimEvidenceLines, 0},
	{shortenTexts, 100},
	{trimTransitions, 10},
	{trimSnapshotRows, 3},
	{trimSnapshots, 1},
	{trimPhases, 10},
	{trimSent, 2},
	{trimMovedFrom, 3},
	{trimOccurrences, 3},
	{trimDeliveries, 5},
	{trimChanges, 3},
	{trimContainers, 3},
	{trimAffected, 5},
	{trimWarnings, 3},
	{trimNodes, 1},
	{trimTraffic, 3},
	{trimProbes, 1},
	{trimWaitReasons, 3},
	{trimUsageHours, 1},
	{trimSameTime, 3},
	{trimPager, 2},
	{trimRelated, 2},
	{trimFixSteps, 3},
	{trimChangedBefore, 1},
	{trimFacts, 0},
	{trimOccurrences, 0},
	{trimDeliveries, 0},
	{trimChanges, 0},
	{trimContainers, 0},
	{trimAffected, 0},
	{trimWarnings, 0},
	{trimNodes, 0},
	{trimTraffic, 0},
	{trimProbes, 0},
	{trimWaitReasons, 0},
	{trimUsageHours, 0},
	{trimSameTime, 0},
	{trimSections, 0},
	{trimTransitions, 0},
	{trimSnapshotRows, 0},
	{trimSnapshots, 0},
	{trimPhases, 0},
	{trimSent, 0},
	{trimMovedFrom, 0},
	{trimPager, 0},
	{trimRelated, 0},
	{trimFixSteps, 0},
	{trimChangedBefore, 0},
	{shortenTexts, 40},
}

// fitDetail cuts d until its JSON fits budget and sets d.Truncated to
// what was left out. A detail that cannot fit is left for writeJSON to
// refuse.
func fitDetail(d *IncidentDetail, budget int) {
	t := DetailTruncation{}
	if d.Truncated != nil {
		t = *d.Truncated
	}
	d.Truncated = &t
	for _, s := range shrinkStages {
		for detailSize(d) > budget {
			if !s.step(d, &t, s.floor) {
				break
			}
		}
	}
	if t == (DetailTruncation{}) {
		d.Truncated = nil
	}
}

func detailSize(d *IncidentDetail) int {
	body, err := json.Marshal(d)
	if err != nil {
		return 0
	}
	return len(body)
}

// keepCount is how many of n items one halving step keeps: half, but
// never under floor. It equals n when there is nothing to cut.
func keepCount(n, floor int) int {
	if n <= floor {
		return n
	}
	return max(floor, n/2)
}

// trimTimeline drops the oldest half of the timeline (kept oldest
// first, so the front).
func trimTimeline(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	n := len(d.Timeline)
	keep := keepCount(n, floor)
	if keep == n {
		return false
	}
	d.Timeline = d.Timeline[n-keep:]
	t.Timeline += n - keep
	return true
}

// trimLogLines drops the earliest half of the captured log lines; the
// last lines before a failure say the most.
func trimLogLines(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	n := len(d.CapturedLogLines)
	keep := keepCount(n, floor)
	if keep == n {
		return false
	}
	d.CapturedLogLines = d.CapturedLogLines[n-keep:]
	t.LogLines += n - keep
	return true
}

// trimEvidenceLines drops half of every section's lines, keeping the
// matched (signature) lines longest: unmatched lines go first, earliest
// first, and matched lines only once no unmatched line is left. The
// matched indexes are renumbered to the lines kept.
func trimEvidenceLines(
	d *IncidentDetail, t *DetailTruncation, floor int,
) bool {
	changed := false
	for i := range d.Evidence {
		s := &d.Evidence[i]
		n := len(s.Lines)
		keep := keepCount(n, floor)
		if keep == n {
			continue
		}
		s.Lines, s.Matched = dropLines(s.Lines, s.Matched, n-keep)
		t.EvidenceLines += n - keep
		changed = true
	}
	return changed
}

// dropLines removes cut lines, unmatched before matched and earliest
// first within each, and returns the lines left with their matched
// indexes renumbered, in line order.
func dropLines(
	lines []string, matched []int, cut int,
) ([]string, []int) {
	isMatched := make([]bool, len(lines))
	for _, m := range matched {
		if m >= 0 && m < len(lines) {
			isMatched[m] = true
		}
	}
	drop := make([]bool, len(lines))
	cut = markEarliest(drop, isMatched, false, cut)
	markEarliest(drop, isMatched, true, cut)
	keptLines := make([]string, 0, len(lines))
	keptMatched := make([]int, 0, len(matched))
	for i, line := range lines {
		if drop[i] {
			continue
		}
		if isMatched[i] {
			keptMatched = append(keptMatched, len(keptLines))
		}
		keptLines = append(keptLines, line)
	}
	return keptLines, keptMatched
}

// markEarliest marks up to cut of the earliest lines whose matched
// flag equals want for dropping, and returns how many are still to cut.
func markEarliest(drop, isMatched []bool, want bool, cut int) int {
	for i := range drop {
		if cut == 0 {
			break
		}
		if isMatched[i] == want {
			drop[i] = true
			cut--
		}
	}
	return cut
}

// trimFacts drops the last half of the correlation's and every
// section's facts: they are listed most telling first.
func trimFacts(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	changed := false
	cut := func(facts []EvidenceFact) []EvidenceFact {
		keep := keepCount(len(facts), floor)
		if keep == len(facts) {
			return facts
		}
		t.EvidenceFacts += len(facts) - keep
		changed = true
		return facts[:keep]
	}
	if d.Correlation != nil {
		d.Correlation.Facts = cut(d.Correlation.Facts)
	}
	for i := range d.Evidence {
		d.Evidence[i].Facts = cut(d.Evidence[i].Facts)
	}
	return changed
}

// trimSections drops the last half of the evidence sections; the
// checks for the root's own kind lead.
func trimSections(d *IncidentDetail, t *DetailTruncation, floor int) bool {
	n := len(d.Evidence)
	keep := keepCount(n, floor)
	if keep == n {
		return false
	}
	d.Evidence = d.Evidence[:keep]
	t.EvidenceSections += n - keep
	return true
}

// shortenTexts cuts every long line, event, fact value, context,
// record and sent text to limit runes. Commands are never cut: a cut
// command would be wrong to copy; a step goes whole instead.
func shortenTexts(d *IncidentDetail, t *DetailTruncation, limit int) bool {
	n := 0
	short := func(s *string) bool {
		cut, ok := shortText(*s, limit)
		if ok {
			*s = cut
			n++
		}
		return ok
	}
	for i := range d.Timeline {
		short(&d.Timeline[i].Text)
	}
	for i := range d.CapturedLogLines {
		short(&d.CapturedLogLines[i])
	}
	for i := range d.Evidence {
		s := &d.Evidence[i]
		for j := range s.Lines {
			short(&s.Lines[j])
		}
		for j := range s.Facts {
			short(&s.Facts[j].Value)
		}
	}
	if d.Correlation != nil {
		for j := range d.Correlation.Facts {
			short(&d.Correlation.Facts[j].Value)
		}
	}
	shortenContext(d, short)
	shortenInsight(d, short)
	shortenJournal(d, short)
	t.Texts += n
	return n > 0
}

// shortText cuts s to limit runes, ending in an ellipsis, and reports
// whether it cut.
func shortText(s string, limit int) (string, bool) {
	if len(s) <= limit {
		return s, false
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s, false
	}
	return strings.TrimSpace(string(runes[:limit-1])) + "…", true
}
