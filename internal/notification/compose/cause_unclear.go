package compose

import (
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
	if p.Cause != nil || !p.CauseUnclear ||
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
// exit facts that follow are then the strongest leads.
func checkedSentences(f caseFacts) []sentence {
	p := f.p
	if p.Cause != nil || rootFinding(p, f.members) != nil ||
		onlyAdvisory(f.members) {
		return nil
	}
	if _, _, byNode := nodeKill(f.members); byNode {
		// The node is named as the cause by the sentences that follow.
		return recentChangeSentence(f)
	}
	text := "Nothing outside it explains this."
	if words := checkedWordsFor(p.Checked); len(words) > 0 {
		text = "Its " + joinWords(words) + " " +
			verb(len(words), "is", "are") + " healthy and unchanged, " +
			"so nothing outside it explains this."
	}
	return append([]sentence{{part: partCause, text: text}},
		recentChangeSentence(f)...)
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
