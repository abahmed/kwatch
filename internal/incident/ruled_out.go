package incident

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// RuledOut is what kwatch checked around an incident and found fine,
// and what it could not check, in plain words for "What kwatch
// checked". Fine lists only what was checked for this incident: the
// objects upstream of its failures that were reached and showed no
// finding and no change (Incident.Checked), and what its caller adds
// (a quiet change history). Also is set when the incident has a cause
// that is only possible: the list then reads "Also ruled out: …".
type RuledOut struct {
	Fine      []string
	Unchecked []string
	Also      bool
}

// Empty reports a RuledOut with nothing to say: no heading is shown.
func (r RuledOut) Empty() bool {
	return len(r.Fine) == 0 && len(r.Unchecked) == 0
}

// checkedItem says one checked kind in words: what it is (Noun, for
// "Also ruled out: its node") and what was found (Clause, for "kwatch
// checked: its node is healthy").
type checkedItem struct {
	kinds  []inventory.Kind
	noun   string
	clause string
}

// checkedItems are the kinds worth a word, in the order they are said.
// ConfigMaps and Secrets share one item. Owners are the workload
// itself and a zone is said through its node, so neither is listed.
var checkedItems = []checkedItem{
	{[]inventory.Kind{kube.KindNode}, "its node", "its node is healthy"},
	{[]inventory.Kind{kube.KindImage}, "its image",
		"its image shows no pull errors and did not change"},
	{[]inventory.Kind{kube.KindRegistry}, "the image registry",
		"the image registry shows no errors"},
	{[]inventory.Kind{kube.KindConfigMap, kube.KindSecret}, "", ""},
	{[]inventory.Kind{kube.KindPVC, kube.KindPV}, "its volumes",
		"its volumes are healthy and did not change"},
	{[]inventory.Kind{kube.KindAccount}, "its service account",
		"its service account is there and did not change"},
	{[]inventory.Kind{kube.KindService}, "the Services it uses",
		"the Services it uses are healthy"},
	{[]inventory.Kind{explain.KindExternalEndpoint}, "its dependencies",
		"its dependencies show no errors"},
}

// RuledOutOf is what kwatch checked around p and what it could not.
// report is the investigation's sections (Decision.Facts.Report or
// Incident.Evidence); quiet, when not empty, is a further clause such
// as "nothing in shop was changed in the last 30 minutes". An incident
// whose cause is more than possible, or a node's own incident (what
// was checked around a node is not what its reader looks at), gets
// nothing.
func RuledOutOf(p Incident, report []Section, quiet string) RuledOut {
	level := rootcause.CertaintyOf(p.Cause)
	if level != rootcause.LevelNone && level != rootcause.LevelPossible {
		return RuledOut{}
	}
	if p.Root.Kind == kube.KindNode {
		return RuledOut{}
	}
	out := RuledOut{Also: level == rootcause.LevelPossible,
		Unchecked: uncheckedOf(report)}
	for _, item := range checkedItems {
		if words := item.words(p.Checked, out.Also); words != "" {
			out.Fine = append(out.Fine, words)
		}
	}
	if quiet != "" && !out.Also {
		out.Fine = append(out.Fine, quiet)
	}
	return out
}

// words is the item's noun or clause when checked holds one of its
// kinds, else "".
func (item checkedItem) words(checked []string, noun bool) string {
	var held []inventory.Kind
	for _, kind := range item.kinds {
		for _, c := range checked {
			if inventory.Kind(c) == kind {
				held = append(held, kind)
				break
			}
		}
	}
	if len(held) == 0 {
		return ""
	}
	if item.noun == "" {
		return configWords(held, noun)
	}
	if noun {
		return item.noun
	}
	return item.clause
}

// configWords says the ConfigMaps and Secrets that were checked.
func configWords(held []inventory.Kind, noun bool) string {
	names := map[inventory.Kind]string{kube.KindConfigMap: "ConfigMaps",
		kube.KindSecret: "Secrets"}
	var parts []string
	for _, kind := range held {
		parts = append(parts, names[kind])
	}
	text := "its " + strings.Join(parts, " and ")
	if noun {
		return text
	}
	return text + " are there and did not change"
}

// uncheckedOf says what the investigation could not read. Only pod
// logs and events are worth a word: they are where a reader looks
// next. A timeout or error is passing and is left out.
func uncheckedOf(report []Section) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range report {
		if s.Status != SectionDenied || seen[s.Check] {
			continue
		}
		seen[s.Check] = true
		if text := deniedWords(s); text != "" {
			out = append(out, text)
		}
	}
	return out
}

// deniedWords says why a denied section could not be read.
func deniedWords(s Section) string {
	what := map[string]string{CheckLogs: "pod logs",
		CheckEvents: "events"}[s.Check]
	switch {
	case what == "":
		return ""
	case s.Reason == ReasonOff:
		return "reading " + what + " is turned off in the configuration"
	}
	return "kwatch cannot read " + what + " here"
}
