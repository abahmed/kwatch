package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// weightConfigVersion ranks the config version fact with the other
// changes: it says which pods read which content.
const weightConfigVersion = weightChange

// configVersionProof is the cause's proof that pods on one version of
// a changed config fail and pods on the other run healthy.
func configVersionProof(f caseFacts) (rootcause.Proof, bool) {
	if f.p.Cause == nil || f.p.Cause.Change == nil {
		return rootcause.Proof{}, false
	}
	for _, item := range f.p.Cause.Proof {
		if item.Supports && (item.Code == rootcause.ProofNewConfigFails ||
			item.Code == rootcause.ProofOldConfigFails) {
			return item, true
		}
	}
	return rootcause.Proof{}, false
}

// hasConfigVersion reports a note whose config version sentence already
// says what the change was, so the change sentence would repeat it.
func hasConfigVersion(f caseFacts) bool {
	_, ok := configVersionProof(f)
	return ok
}

// configVersionSentences say when the config changed, what changed and
// who changed it, and which pods fail: "Config map api-config changed
// ten minutes ago (keys: DB_HOST, TIMEOUT; by alice); the two pods
// started since then fail, the three older ones are healthy."
func configVersionSentences(f caseFacts) []sentence {
	proof, ok := configVersionProof(f)
	if !ok {
		return nil
	}
	change := *f.p.Cause.Change
	text := shortName(f.p.Cause.Root) + " changed " +
		humanDuration(f.now.Sub(change.At)) + " ago" +
		configChangeDetail(change.Actor, change.Fields) + "; " +
		configVersionSplit(proof)
	return []sentence{{part: partProof, weight: weightConfigVersion,
		text: endSentence(upperFirst(text))}}
}

// configChangeDetail is " (keys: DB_HOST, TIMEOUT; by alice)": what
// changed, by key name only, and who changed it, when known.
func configChangeDetail(actor string, fields []inventory.FieldChange) string {
	var parts []string
	var keys []string
	for _, field := range fields {
		if key, ok := strings.CutPrefix(field.Path, "data."); ok {
			keys = append(keys, key)
		}
	}
	if len(keys) > 0 {
		parts = append(parts, "keys: "+strings.Join(
			limit(keys, maxNamed), ", ")+more(len(keys), maxNamed))
	}
	if who := person(actor); who != "" {
		parts = append(parts, "by "+who)
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, "; ") + ")"
}

// configVersionSplit says which side of the change fails and which
// runs healthy.
func configVersionSplit(p rootcause.Proof) string {
	if p.Code == rootcause.ProofNewConfigFails {
		return "the " + verbPhrase(p.Count, "pod", "started since then",
			"fails", "fail") + ", the " + verbPhrase(p.Total, "older one",
			"", "is healthy", "are healthy")
	}
	return "the " + verbPhrase(p.Count, "pod", "still on the old version",
		"fails", "fail") + ", the " + verbPhrase(p.Total, "one",
		"started since then", "is healthy", "are healthy")
}

// verbPhrase writes "two pods started since then fail" or "pod started
// since then fails": the count (left out for one), the noun, an
// optional qualifier and the verb that agrees with the count.
func verbPhrase(n int, noun, qualifier, one, many string) string {
	parts := []string{noun}
	if n != 1 {
		parts = []string{numberWord(n), noun + "s"}
	}
	if qualifier != "" {
		parts = append(parts, qualifier)
	}
	return strings.Join(parts, " ") + " " + verb(n, one, many)
}
