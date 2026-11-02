package compose

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// This file writes the "Also grouped" line: the other findings that
// joined an incident, counted by type. All of its wording lives here so
// a wording change touches one file.

const (
	groupedPrefix = "Also grouped: "
	// groupedMaxTypes is how many types the line names before it says
	// how many more there are.
	groupedMaxTypes = 4
)

// groupedLabel is a type of finding in words, singular and plural.
type groupedLabel struct{ one, many string }

var (
	labelProbe    = groupedLabel{"probe failure", "probe failures"}
	labelRollout  = groupedLabel{"rollout stall", "rollout stalls"}
	labelCrash    = groupedLabel{"crash loop", "crash loops"}
	labelOOM      = groupedLabel{"out-of-memory kill", "out-of-memory kills"}
	labelSchedule = groupedLabel{"scheduling failure", "scheduling failures"}
	labelPull     = groupedLabel{"image pull failure", "image pull failures"}
	labelNode     = groupedLabel{"node problem", "node problems"}
	labelMissing  = groupedLabel{"missing reference", "missing references"}
	labelCreate   = groupedLabel{"failed pod creation", "failed pod creations"}
	labelNoEnds   = groupedLabel{"service without endpoints",
		"services without endpoints"}
	labelDown = groupedLabel{"unavailable workload",
		"unavailable workloads"}
	labelPending  = groupedLabel{"pending pod", "pending pods"}
	labelThrottle = groupedLabel{"CPU throttling alert",
		"CPU throttling alerts"}
	labelNotReady = groupedLabel{"not-ready pod", "not-ready pods"}
	labelEvicted  = groupedLabel{"eviction", "evictions"}
	labelNetwork  = groupedLabel{"network problem", "network problems"}
)

// groupedExact label a mode as a whole. These modes share a family with
// others that mean something else (Unavailable.APIServer is not an
// unavailable workload).
var groupedExact = map[detection.Mode]groupedLabel{
	detection.ModeUnavailable:             labelDown,
	detection.ModeNoEndpoints:             labelNoEnds,
	detection.ModeFailedCreate:            labelCreate,
	detection.ModeEvicted:                 labelEvicted,
	detection.ModeCPUThrottled:            labelThrottle,
	detection.ModeNetworkErrors:           labelNetwork,
	detection.ModeStatefulSetRolloutStuck: labelRollout,
}

// groupedLabels name the failure families that have a label of their
// own. A family not listed reads as "<words> alert".
var groupedLabels = map[detection.Mode]groupedLabel{
	detection.ModeProbe:                       labelProbe,
	detection.ModeRolloutStuck:                labelRollout,
	detection.ModeCrashLoop:                   labelCrash,
	detection.ModeOOMKilled:                   labelOOM,
	detection.ModeUnschedulable:               labelSchedule,
	detection.ModeSchedulingGated:             labelSchedule,
	detection.ModeImagePull:                   labelPull,
	detection.ModeMemoryPressure:              labelNode,
	detection.ModeDiskPressure:                labelNode,
	detection.ModePIDPressure:                 labelNode,
	detection.ModeNetworkUnavailable:          labelNode,
	detection.ModeDraining:                    labelNode,
	detection.ModeKubeletUnreachable.Family(): labelNode,
	detection.ModeHeartbeatStale.Family():     labelNode,
	detection.ModeNodeCondition.Family():      labelNode,
	detection.ModeNetwork:                     labelNetwork,
}

func modeOf(f detection.Finding) detection.Mode {
	if f.Mode != "" {
		return f.Mode
	}
	return detection.ModeFor(f.Reason)
}

// groupedLabelOf is the label of a finding: by its kind when it is a
// node's, else by its mode, else by its failure family.
func groupedLabelOf(f detection.Finding) groupedLabel {
	if f.Entity.Kind == kube.KindNode {
		return labelNode
	}
	mode := modeOf(f)
	if l, ok := groupedExact[mode]; ok {
		return l
	}
	if mode.Family() == "Missing" || strings.HasSuffix(string(mode), "Missing") {
		return labelMissing
	}
	if l, ok := podMode(f, mode); ok {
		return l
	}
	if l, ok := groupedLabels[mode.Family()]; ok {
		return l
	}
	words := modeWords(mode.Family())
	return groupedLabel{words + " alert", words + " alerts"}
}

// podMode labels the modes whose noun depends on what the finding is
// about: a Pending phase is also reported for other objects, and only a
// pod or container is a "not-ready pod".
func podMode(f detection.Finding, mode detection.Mode) (groupedLabel, bool) {
	isPod := f.Entity.Kind == kube.KindPod ||
		f.Entity.Kind == kube.KindContainer
	switch {
	case mode == detection.ModePending && isPod:
		return labelPending, true
	case mode == detection.ModeNotReady && isPod:
		return labelNotReady, true
	}
	return groupedLabel{}, false
}

// modeWords spells a mode family as lowercase words: "NoEndpoints" is
// "no endpoints" and "CPUThrottled" is "cpu throttled". A run of
// capitals is one word. A raw identifier is never shown.
func modeWords(m detection.Mode) string {
	runes := []rune(string(m))
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && wordStart(runes, i) {
			b.WriteRune(' ')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// wordStart reports a capital at i that begins a new word: after a
// lowercase letter, or the last capital of a run that a lowercase
// letter follows ("CPUThrottled" splits before the T).
func wordStart(runes []rune, i int) bool {
	if unicode.IsLower(runes[i-1]) {
		return true
	}
	return unicode.IsUpper(runes[i-1]) && i+1 < len(runes) &&
		unicode.IsLower(runes[i+1])
}

// groupedSentences is the line that counts the other findings grouped
// under the incident, by type. Not counted: the lead's own finding,
// advisories, and findings of the lead's failure family, which the lead
// (or a storm sentence, which names the lead's state) already says.
// Two or more others are shown; one alone is shown unless it is only a
// symptom of the lead's own workload.
func groupedSentences(f caseFacts) []sentence {
	others := groupedOthers(f)
	if len(others) == 0 ||
		(len(others) == 1 && ownSymptom(f.p.Root, others[0])) {
		return nil
	}
	counts := map[groupedLabel]int{}
	for _, m := range others {
		counts[groupedLabelOf(m)]++
	}
	return []sentence{{part: partGrouped, text: groupedText(counts)}}
}

// groupedOthers are the members the line may count.
func groupedOthers(f caseFacts) []detection.Finding {
	var out []detection.Finding
	for _, m := range f.members {
		if m.Advisory || (f.ok && (m.Key() == f.lead.Key() ||
			modeOf(m).Family() == modeOf(f.lead).Family())) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// ownSymptom reports a symptom on the root's own workload, such as its
// Service without endpoints: it restates the failure, it adds none.
func ownSymptom(root inventory.EntityID, f detection.Finding) bool {
	return f.Symptom && ownedBy(root, f)
}

func groupedText(counts map[groupedLabel]int) string {
	labels := make([]groupedLabel, 0, len(counts))
	for l := range counts {
		labels = append(labels, l)
	}
	sort.Slice(labels, func(i, j int) bool {
		if counts[labels[i]] != counts[labels[j]] {
			return counts[labels[i]] > counts[labels[j]]
		}
		return labels[i].many < labels[j].many
	})
	var parts []string
	for i, l := range labels {
		if i == groupedMaxTypes {
			break
		}
		parts = append(parts, countedLabel(counts[l], l))
	}
	text := strings.Join(parts, ", ")
	if more := len(labels) - groupedMaxTypes; more > 0 {
		noun := "types"
		if more == 1 {
			noun = "type"
		}
		text += fmt.Sprintf(", and %d more %s", more, noun)
	}
	return groupedPrefix + text
}

func countedLabel(n int, l groupedLabel) string {
	if n == 1 {
		return "1 " + l.one
	}
	return fmt.Sprintf("%d %s", n, l.many)
}

// ownedBy reports a finding of the root's own workload: its pod or
// container, or a Service of the same name.
func ownedBy(root inventory.EntityID, f detection.Finding) bool {
	e := f.Entity
	if e == root {
		return true
	}
	return e.Namespace == root.Namespace && root.Name != "" &&
		(e.Name == root.Name || strings.HasPrefix(e.Name, root.Name+"-") ||
			strings.HasPrefix(e.Name, root.Name+"/"))
}
