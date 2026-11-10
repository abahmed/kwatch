package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// unclearSentences follow the lead when no cause reached the confidence
// floor although the failure could have one upstream: the root is the
// failing object itself and candidates outside it were weighed and
// dropped. Without a root finding checkedSentences speak instead, and
// a cause, however hedged, is a cause.
func unclearSentences(f caseFacts) []sentence {
	p := f.p
	if p.Cause != nil || !p.CauseUnclear || toldCause(f) ||
		rootFinding(p, f.members) == nil {
		return nil
	}
	return []sentence{{part: partCause, text: "The cause is not clear yet."}}
}

// checkedWords name, for a reader, what an upstream kind is to the
// failing workload. Kinds not listed are not worth a word: owners are
// the workload itself, and a zone is said through its node.
var checkedWords = map[inventory.Kind]string{
	kube.KindNode:                "node",
	kube.KindImage:               "image",
	kube.KindRegistry:            "registry",
	kube.KindConfigMap:           "configuration",
	kube.KindSecret:              "configuration",
	kube.KindPVC:                 "volumes",
	kube.KindPV:                  "volumes",
	kube.KindAccount:             "service account",
	kube.KindService:             "services",
	explain.KindExternalEndpoint: "dependencies",
}

// checkedSentences follow a lead that has no cause and no finding of
// the root's own: a workload failing without anything outside it to
// blame. Instead of leaving the reader with no cause, they say what was
// checked and found healthy and unchanged. The quoted output and the
// exit facts that follow are then the strongest leads. They say what
// points to a cause, not that nothing explains the failure: the checks
// cover the upstream kinds the solver reached, and a candidate it weighed
// and dropped (CauseUnclear) or could not see (Unverified) is not ruled
// out.
func checkedSentences(f caseFacts) []sentence {
	p := f.p
	if p.Cause != nil || rootFinding(p, f.members) != nil ||
		onlyAdvisory(f.members) {
		return nil
	}
	recent := recentChangeSentence(f)
	if !checkedSaysMore(f) {
		return recent
	}
	next := "."
	if lookAtTheApp(f, len(recent) > 0) {
		next = ", so look at the app itself."
	}
	text := "Cause not found yet: nothing outside it points to one" + next
	if words := checkedWordsFor(p.Checked); len(words) > 0 {
		text = "Cause not found yet: its " + joinWords(words) + " " +
			verb(len(words), "is", "are") + " healthy and unchanged" + next
	}
	return append([]sentence{{part: partCause, text: text}}, recent...)
}

// checkedSaysMore reports a message the "Cause not found yet" sentence
// adds to. It says nothing when the message names a cause or where to
// fix it already: a node that killed the pods, a caused-by line, a fix
// place, a CPU the image is not built for. Nor for a node: what was
// checked around a node is not what a reader of a node alert looks at.
func checkedSaysMore(f caseFacts) bool {
	if _, _, byNode := nodeKill(f.members); byNode {
		return false
	}
	if toldCause(f) || f.p.Root.Kind == kube.KindNode {
		return false
	}
	if _, ok := f.fixPlace(); ok {
		return false
	}
	_, arch := archLead(f)
	return !arch
}

// appModes are the lead modes whose failure is the app's own when
// nothing outside it points to a cause: it crashes, exits, errors,
// restarts or fails its probes.
var appModes = []detection.Mode{detection.ModeCrashLoop,
	detection.ModeError, detection.ModeRestarting, detection.ModeProbe,
	detection.ModeExit}

// lookAtTheApp reports a failure whose next place to look is the app
// itself: a workload whose lead is one of appModes, with nothing kwatch
// could not see (Unverified), no shared factor a Pro analysis found
// (ProCause) and no recent change (changed) to look at first.
func lookAtTheApp(f caseFacts, changed bool) bool {
	p := f.p
	if changed || f.changeSeen || p.ProCause || len(p.Unverified) > 0 {
		return false
	}
	if !isWorkloadKind(p.Root.Kind) && p.Root.Kind != kube.KindPod {
		return false
	}
	mode := p.Mode
	if f.ok {
		mode = findingMode(f.lead)
	}
	for _, m := range appModes {
		if mode == m || strings.HasPrefix(string(mode), string(m)+".") {
			return true
		}
	}
	return false
}

// onlyAdvisory reports members that are all configuration risks: there
// is no failure to explain.
func onlyAdvisory(members []detection.Finding) bool {
	for _, m := range members {
		if !m.Advisory {
			return false
		}
	}
	return len(members) > 0
}

// checkedWordsFor words the checked kinds, each word once, in the
// kinds' sorted order.
func checkedWordsFor(kinds []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, kind := range kinds {
		word, ok := checkedWords[inventory.Kind(kind)]
		if !ok || seen[word] {
			continue
		}
		seen[word] = true
		out = append(out, word)
	}
	return out
}
