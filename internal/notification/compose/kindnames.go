package compose

import (
	"regexp"

	"github.com/abahmed/kwatch/internal/inventory"
)

// respell gives custom resources the kind spelling their CRD declares:
// "datadogagent datadog" becomes "DatadogAgent datadog". Messages name
// kinds in lower case, as the entity kind is; the declared spellings
// come with the decision (Decision.Facts.KindNames), read from the
// entities. Actions are left alone, since a command is typed as written.
//
// It works on the finished sentences, not where the kind word is made,
// because the writers build names in many places (lead, impact, cause,
// steps) that have no access to the decision. Only a kind word that is
// followed by a name is rewritten: "datadogagent datadog".
func respell(
	sentences []sentence, names map[inventory.Kind]string,
) []sentence {
	if len(names) == 0 {
		return sentences
	}
	patterns := make(map[*regexp.Regexp]string, len(names))
	for kind, declared := range names {
		patterns[kindBeforeName(kind)] = declared
	}
	out := make([]sentence, len(sentences))
	copy(out, sentences)
	for i := range out {
		if out[i].part == partAction {
			continue
		}
		out[i].text = outsideQuotes(out[i].text, func(part string) string {
			for pattern, declared := range patterns {
				part = pattern.ReplaceAllString(part, declared+"$1")
			}
			return part
		})
	}
	return out
}

// kindBeforeName matches a kind in lower case, as a whole word, followed
// by the first character of a name. The character is kept ($1).
func kindBeforeName(kind inventory.Kind) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(string(kind)) +
		`\b( \S)`)
}
