package compose

import (
	"sort"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// minTimelineSteps is the fewest steps worth a line of their own: with
// two, the cause sentence already says it.
const minTimelineSteps = 3

// maxTimelineNames is how many workloads one step names before it
// counts the rest.
const maxTimelineNames = 2

// maxTimelineLength is the length of the long text past which the line
// is left out of the message; the dashboard keeps it.
const maxTimelineLength = 600

// factTimeline is the fact the timeline line states, so it can be found
// again and left out of a message that is already long.
const factTimeline factKey = "timeline"

// eventWords are the observed events of a finding's mode, in the words
// of the timeline line: what was seen, never why.
var eventWords = map[detection.Mode]string{
	detection.ModeCrashLoop:      "crash",
	detection.ModeRestarting:     "crash",
	detection.ModeOOMKilled:      "out of memory",
	detection.ModeEvicted:        "evicted",
	detection.ModeDiskPressure:   "disk pressure",
	detection.ModeMemoryPressure: "memory pressure",
	detection.ModePIDPressure:    "pid pressure",
	detection.ModeNotReady:       "not ready",
	detection.ModeUnavailable:    "unavailable",
	detection.ModeNoEndpoints:    "no ready endpoints",
	detection.ModeImagePull:      "image pull failing",
	detection.ModeUnschedulable:  "unschedulable",
}

// timelineStep is one line of the chain: the workloads or objects that
// began failing in one minute with one observed event.
type timelineStep struct {
	minute string
	event  string
	names  []string
}

// chainLineSentences show a failure chain in the order each step was
// first seen failing, as one line: "Timeline: 14:02 n3 disk pressure →
// 14:03 redis evicted → 14:03 checkout, cart crash." It is the order of
// onsets, not a claim of cause. A chain of fewer than three steps has
// no line. Failures one step past the limit are named, so the reader
// knows the chain was cut and not finished.
func chainLineSentences(f caseFacts) []sentence {
	cause := f.p.Cause
	if cause == nil {
		return nil
	}
	steps := timelineSteps(f, cause)
	if len(steps) < minTimelineSteps {
		return nil
	}
	parts := make([]string, len(steps))
	for i, step := range steps {
		parts[i] = step.words()
	}
	text := "Timeline: " + strings.Join(parts, " → ") + "."
	if beyond := newBeyond(cause); len(beyond) > 0 {
		text += " " + beyondWords(beyond)
	}
	return []sentence{{part: partConsequence, weight: 0.1,
		fact: factTimeline, text: text}}
}

// words is "14:03 checkout, cart crash".
func (s timelineStep) words() string {
	names := s.names
	rest := 0
	if len(names) > maxTimelineNames {
		rest = len(names) - maxTimelineNames
		names = names[:maxTimelineNames]
	}
	text := strings.Join(names, ", ")
	if rest > 0 {
		text += " +" + strconv.Itoa(rest)
	}
	if s.minute != "" {
		text = s.minute + " " + text
	}
	return text + " " + s.event
}

// timelineSteps orders the chain's hops by the time each began and joins
// the hops of one minute and one event into a step. A hop with no known
// time keeps its place in the chain and names no minute.
func timelineSteps(
	f caseFacts, cause *rootcause.CauseRecord,
) []timelineStep {
	hops := append([]rootcause.Hop(nil), cause.Hops...)
	sort.SliceStable(hops, func(i, j int) bool {
		a, b := hops[i].Began, hops[j].Began
		return !a.IsZero() && !b.IsZero() && a.Before(b)
	})
	var steps []timelineStep
	named := map[string]bool{}
	for _, hop := range hops {
		// The Service of a workload that failed in the same minute is
		// that workload again; a minute later it is news.
		key := hop.Entity.Name + "@" + hop.Began.UTC().Format("15:04")
		if named[key] {
			continue
		}
		named[key] = true
		step := timelineStep{event: hopEvent(f, hop.Entity),
			names: []string{stepName(hop.Entity)}}
		if !hop.Began.IsZero() {
			step.minute = hop.Began.UTC().Format("15:04")
		}
		last := len(steps) - 1
		if last >= 0 && steps[last].minute == step.minute &&
			steps[last].event == step.event {
			if !containsName(steps[last].names, step.names[0]) {
				steps[last].names = append(steps[last].names, step.names[0])
			}
			continue
		}
		steps = append(steps, step)
	}
	return steps
}

func containsName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// hopEvent is the observed event of a hop: the mode of the most severe
// finding of the entity or of its pods and containers, else "failing".
func hopEvent(f caseFacts, hop inventory.EntityID) string {
	var best *detection.Finding
	for i, m := range f.members {
		if !belongsTo(m.Entity, hop) {
			continue
		}
		if _, known := eventWords[findingMode(m)]; !known {
			continue
		}
		if best == nil || m.Severity > best.Severity {
			best = &f.members[i]
		}
	}
	if best == nil {
		return "failing"
	}
	return eventWords[findingMode(*best)]
}

// belongsTo reports a finding about the hop, or about a pod or container
// named after it (a workload's pods carry its name as their prefix).
func belongsTo(entity, hop inventory.EntityID) bool {
	if entity == hop {
		return true
	}
	return entity.Namespace == hop.Namespace &&
		strings.HasPrefix(entity.Name, hop.Name+"-")
}

// newBeyond are the failures past the end of the chain that the chain
// does not already name: the Service of the workload that ends it is
// that workload again, so it is no news that "service web" also fails
// after "web".
func newBeyond(cause *rootcause.CauseRecord) []rootcause.Hop {
	named := map[string]bool{}
	for _, hop := range cause.Hops {
		named[hop.Entity.Name] = true
	}
	var out []rootcause.Hop
	for _, hop := range cause.Beyond {
		if !named[hop.Entity.Name] {
			out = append(out, hop)
		}
	}
	return out
}

// hopWords names one hop with the time it began: "api (10:01)".
func hopWords(hop rootcause.Hop) string {
	name := shortName(hop.Entity)
	if hop.Began.IsZero() {
		return name
	}
	return name + " (" + clock(hop.Began) + ")"
}

// beyondWords says what lies past the end of a cut chain.
func beyondWords(beyond []rootcause.Hop) string {
	names := make([]string, 0, len(beyond))
	for _, hop := range beyond {
		names = append(names, hopWords(hop))
	}
	verb := " also fails"
	if len(names) > 1 {
		verb = " also fail"
	}
	return "Beyond that, " + strings.Join(names, " and ") + verb +
		"; kwatch follows a chain at most " +
		strconv.Itoa(explain.ChainMaxHops) +
		" steps, so it is reported on its own."
}

// trimTimeline leaves the timeline line out of a message whose long text
// would pass maxTimelineLength with it. The dashboard keeps the line.
func trimTimeline(sentences []sentence) []sentence {
	length := 0
	for _, s := range sentences {
		length += len(s.text) + 1
	}
	if length <= maxTimelineLength {
		return sentences
	}
	var out []sentence
	for _, s := range sentences {
		if s.fact != factTimeline {
			out = append(out, s)
		}
	}
	return out
}

// stepName names a hop: a node by its bare name, as an operator knows it,
// anything else as the messages name it.
func stepName(id inventory.EntityID) string {
	if id.Kind == kube.KindNode {
		return id.Name
	}
	return shortName(id)
}

// findingMode is the mode of a finding, from its reason when it has none.
func findingMode(f detection.Finding) detection.Mode {
	if f.Mode != "" {
		return f.Mode
	}
	return detection.ModeFor(f.Reason)
}

// completedChainSentences are the timeline line of an update that
// completes the chain: its last step began after the incident was
// announced, so no earlier message could have told it whole.
func completedChainSentences(f caseFacts) []sentence {
	cause := f.p.Cause
	if cause == nil || f.p.Announced.IsZero() {
		return nil
	}
	for _, hop := range cause.Hops {
		if hop.Began.After(f.p.Announced) {
			return chainLineSentences(f)
		}
	}
	return nil
}
