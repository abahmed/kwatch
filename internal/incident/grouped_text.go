package incident

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// maxGroupedError is the most runes of a shared error line the
// grouping sentence quotes.
const maxGroupedError = 160

// chainNames is the most objects of a failure chain the sentence names.
const chainNames = 4

// kindWords name a kind in a sentence; a kind not listed is named as
// the model spells it.
var kindWords = map[inventory.Kind]string{
	kube.KindService:         "service",
	kube.KindConfigMap:       "ConfigMap",
	kube.KindSecret:          "Secret",
	kube.KindPVC:             "volume claim",
	kube.KindNodePool:        "node pool",
	kube.KindHelmRelease:     "release",
	kube.KindAccount:         "service account",
	kube.KindStorageClass:    "storage class",
	kube.KindIngress:         "ingress",
	kube.KindMutatingWebhook: "webhook",
	kube.KindValidatingHook:  "webhook",
}

// dependencyVerbs say how the members use a dependency they share.
var dependencyVerbs = map[inventory.Kind]string{
	kube.KindService:             "call",
	explain.KindExternalEndpoint: "call",
	kube.KindConfigMap:           "read",
	kube.KindSecret:              "use",
	kube.KindPVC:                 "mount",
	kube.KindAccount:             "run as",
	kube.KindRegistry:            "pull from",
	kube.KindStorageClass:        "use",
}

// objectWords is "node n1", "service db" or, for an address or an
// object named after its kind ("cluster-dns"), the name alone.
func objectWords(id inventory.EntityID) string {
	word := kindWord(id.Kind)
	if id.Kind == explain.KindExternalEndpoint || word == id.Name ||
		string(id.Kind) == id.Name {
		return id.Name
	}
	return word + " " + id.Name
}

// kindWord names a kind in a sentence.
func kindWord(kind inventory.Kind) string {
	if word, ok := kindWords[kind]; ok {
		return word
	}
	return string(kind)
}

// countWords is "both pods" for two, "all 12 pods" for more.
func countWords(n int, noun string) string {
	if n == 2 {
		return "both " + noun
	}
	return "all " + strconv.Itoa(n) + " " + noun
}

// podsOrUnits counts the pods when there are several, else the groups.
func podsOrUnits(u memberUnits) string {
	if u.pods >= 2 {
		return countWords(u.pods, "pods")
	}
	return countWords(len(u.groups), u.word())
}

// nodeState is ", which is not ready" for a node the model holds as
// not ready, "" otherwise.
func nodeState(model inventory.Reader, node inventory.EntityID) string {
	if model == nil {
		return ""
	}
	e, ok := model.Entity(node)
	if !ok {
		return ""
	}
	a, ok := e.Attribute(kube.AttrReady)
	if ready, known := a.Value.AsBool(); ok && known && !ready {
		return ", which is not ready"
	}
	return ""
}

// mergedNodes is ", where 3 nodes failed" for a place outage that
// merged several node failures, "" otherwise.
func mergedNodes(p Incident) string {
	n := 0
	for _, id := range p.Merged {
		if id.Kind == kube.KindNode {
			n++
		}
	}
	if n < 2 {
		return ""
	}
	return ", where " + strconv.Itoa(n) + " nodes failed"
}

// causeGrouping is the sentence for a grouping that rests on p's
// stated cause.
func causeGrouping(p Incident, u memberUnits) (string, string) {
	c := p.Cause
	if line := sharedLine(c.SharedError); line != "" {
		return GroupedError, endStop("Grouped because " + u.noun() +
			" log the same error: " + line)
	}
	all := countWords(len(u.groups), u.word())
	switch c.Root.Kind {
	case kube.KindImage:
		if c.Change != nil {
			return GroupedImage, "Grouped because " + all +
				" were changed to image " + c.Root.Name + "."
		}
		return GroupedImage, "Grouped because " + all + " run image " +
			c.Root.Name + "."
	case kube.KindHelmRelease:
		return GroupedRelease, "Grouped because " + all +
			" are deployed by release " + c.Root.Name + "."
	}
	if verb, ok := dependencyVerbs[c.Root.Kind]; ok {
		return GroupedDependency, "Grouped because " + all + " " + verb +
			" " + objectWords(c.Root) + "."
	}
	if chain := chainWords(c.Hops); chain != "" {
		return GroupedChain, "Grouped because the failure spreads " +
			"along one chain: " + chain + "."
	}
	return GroupedCause, "Grouped because kwatch traced " + all +
		" to " + objectWords(c.Root) + "."
}

// sharedLine is a shared error line made safe to quote: redacted, on
// one line, without a closing stop, and cut after a whole word with
// "…" past maxGroupedError runes.
func sharedLine(text string) string {
	text = strings.Join(strings.Fields(redact.Evidence(text)), " ")
	text = strings.TrimRight(text, ".")
	if utf8.RuneCountInString(text) <= maxGroupedError {
		return text
	}
	cut := string([]rune(text)[:maxGroupedError])
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, ".,;: ") + " …"
}

// endStop ends a sentence with a stop, unless it ends with "…".
func endStop(text string) string {
	if strings.HasSuffix(text, "…") {
		return text
	}
	return text + "."
}

// chainWords is "a → b → c" for a chain of three or more objects, the
// last ones dropped past chainNames.
func chainWords(hops []rootcause.Hop) string {
	if len(hops) < 3 {
		return ""
	}
	var names []string
	for i, h := range hops {
		if i == chainNames {
			names = append(names, "…")
			break
		}
		names = append(names, objectWords(h.Entity))
	}
	return strings.Join(names, " → ")
}
