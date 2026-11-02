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
)

// groupedLabels name the failure families that have a label of their
// own. A family not listed reads as "<words> alert".
var groupedLabels = map[detection.Mode]groupedLabel{
	detection.ModeProbe:                       labelProbe,
	detection.ModeRolloutStuck:                labelRollout,
	detection.ModeStatefulSetRolloutStuck:     labelRollout,
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
}

// groupedLabelOf is the label of a finding: by its kind when it is a
// node's, else by its failure family.
func groupedLabelOf(f detection.Finding) groupedLabel {
	if f.Entity.Kind == kube.KindNode {
		return labelNode
	}
	mode := f.Mode
	if mode == "" {
		mode = detection.ModeFor(f.Reason)
	}
	if mode == detection.ModeStatefulSetRolloutStuck {
		return labelRollout
	}
	if l, ok := groupedLabels[mode.Family()]; ok {
		return l
	}
	words := modeWords(mode.Family())
	return groupedLabel{words + " alert", words + " alerts"}
}

// modeWords spells a mode family as lowercase words: "NoEndpoints" is
// "no endpoints". A raw identifier is never shown.
func modeWords(m detection.Mode) string {
	var b strings.Builder
	for i, r := range string(m) {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteRune(' ')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// groupedSentences is the line that counts the other findings grouped
// under the incident, by type. The lead's own finding and advisories are
// not counted; nothing grouped writes nothing.
func groupedSentences(f caseFacts) []sentence {
	counts := map[groupedLabel]int{}
	for _, m := range f.members {
		if m.Advisory || (f.ok && m.Key() == f.lead.Key()) {
			continue
		}
		label := groupedLabelOf(m)
		if f.ok && label == groupedLabelOf(f.lead) && ownedBy(f.p.Root, m) {
			continue
		}
		counts[label]++
	}
	if len(counts) == 0 {
		return nil
	}
	return []sentence{{part: partGrouped, text: groupedText(counts)}}
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

// ownedBy reports a finding of the root's own pod or container: the
// root's failure seen one level down, which the lead already says.
func ownedBy(root inventory.EntityID, f detection.Finding) bool {
	e := f.Entity
	if e == root {
		return true
	}
	return e.Namespace == root.Namespace && root.Name != "" &&
		(strings.HasPrefix(e.Name, root.Name+"-") ||
			strings.HasPrefix(e.Name, root.Name+"/"))
}
