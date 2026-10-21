package explain

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// maxHintNames bounds the look-alike workloads a note names.
const maxHintNames = 3

// minNameToken is the shortest word of a workload's name that may stand
// for the API group of a finalizer.
const minNameToken = 4

func init() {
	// The note comes last and never moves a score.
	scorers = append(scorers, scorer{"finalizer", scoreFinalizerHint})
}

// scoreFinalizerHint notes the workloads named like the controller of a
// finalizer that no evidence ties to it. A name proves nothing, so the
// finalizer stays unhandled; the note lets the message say what kwatch
// saw and could not confirm, and it weighs nothing.
func scoreFinalizerHint(v *view, c *candidate) outcome {
	if c.id.Kind != KindFinalizer {
		return outcome{}
	}
	effects := c.others()
	if len(effects) == 0 || len(v.handlersOf(effects[0])) > 0 {
		return outcome{}
	}
	names := v.lookAlikes(v.subjectOf(effects))
	if len(names) == 0 {
		return outcome{}
	}
	return outcome{code: rootcause.ProofNamedLikeHandler, fields: names,
		text: strings.Join(names, ", ") +
			" are named like the controller, nothing says they serve it"}
}

// lookAlikes lists, cluster-wide and at most maxHintNames, the workloads
// with a word in their name that is the first label of one of the
// groups: "widgets-controller" for widgets.example.com. Each is "kind
// namespace/name".
func (v *view) lookAlikes(s finalizerSubject) []string {
	var out []string
	for _, kind := range handlerKinds {
		for _, w := range v.s.Model.Entities(kind) {
			if namedAfter(w, s.groups) {
				out = append(out, workloadText(w))
			}
		}
	}
	sort.Strings(out)
	if len(out) > maxHintNames {
		out = out[:maxHintNames]
	}
	return out
}

// workloadText is "deployment ns/name".
func workloadText(id inventory.EntityID) string {
	return string(id.Kind) + " " + id.Namespace + "/" + id.Name
}

// namedAfter reports whether a word of the workload's name is the first
// label of one of the groups.
func namedAfter(w inventory.EntityID, groups []string) bool {
	words := strings.FieldsFunc(w.Name, func(r rune) bool {
		return r == '-' || r == '.' || r == '_'
	})
	for _, group := range groups {
		first, _, _ := strings.Cut(group, ".")
		if len(first) >= minNameToken && hasText(words, first) {
			return true
		}
	}
	return false
}
