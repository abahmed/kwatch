package explain

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// causeOf turns a scored candidate into a stated cause.
func (v *view) causeOf(s scored) Cause {
	best := s.c.bestMatch()
	return Cause{
		Root: s.c.id, Row: best.match.row.Name, Mode: best.match.causeMode,
		Covers: sortedKeys(s.c.covers), Chain: best.chain,
		Confidence: s.confidence, Level: levelOf(s.confidence),
		Contributions: s.contributions, Changes: v.changesOf(s.c.id),
		Summary: summaryOf(s.c.id, best.match.causeMode, len(s.c.covers)),
	}
}

// summaryOf states a cause as "node n1 (MemoryPressure) explains 3
// failures".
func summaryOf(
	root inventory.EntityID, mode detection.Mode, covers int,
) string {
	out := string(root.Kind) + " " + root.Name
	if mode != "" {
		out += " (" + string(mode) + ")"
	}
	if covers == 1 {
		return out + " explains 1 failure"
	}
	return out + " explains " + strconv.Itoa(covers) + " failures"
}

// traceOf lists the candidates of an area and why the others were
// dropped. A candidate that was viable but not chosen is redundant:
// the chosen causes already cover its failures.
func (v *view) traceOf(
	ranked []scored, inArea map[inventory.EntityID]bool,
	rejected map[inventory.EntityID]rejectNote,
	chosen map[inventory.EntityID]bool,
) Trace {
	var trace Trace
	scoredIDs := map[inventory.EntityID]bool{}
	for _, s := range ranked {
		scoredIDs[s.c.id] = true
		if !touches(s, inArea) {
			continue
		}
		trace.Candidates = append(trace.Candidates, v.causeOf(s))
		if note, ok := rejected[s.c.id]; ok {
			trace.Rejected = append(trace.Rejected,
				Rejection{Root: s.c.id, Reason: note.reason})
		} else if !chosen[s.c.id] {
			trace.Rejected = append(trace.Rejected, Rejection{
				Root:   s.c.id,
				Reason: "redundant: the chosen causes cover its failures",
			})
		}
	}
	for _, id := range sortedKeys(rejected) {
		note := rejected[id]
		if !scoredIDs[id] && inArea[note.effect] {
			trace.Rejected = append(trace.Rejected,
				Rejection{Root: id, Reason: note.reason})
		}
	}
	return trace
}
