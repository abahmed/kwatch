package compose

import "strings"

// unverifiedConsequences say what a blind spot leaves open, by the plain
// kind name rootcause.UnverifiedScope starts with.
var unverifiedConsequences = map[string]string{
	"secrets":          "a changed secret",
	"config maps":      "a changed config map",
	"service accounts": "a missing service account",
	"volume claims":    "a failing volume claim",
	"network policies": "a blocking network policy",
	"nodes":            "a node problem",
}

// unverifiedLine states in one sentence what kwatch could not check, as
// in "I can't see secrets in billing, so a changed secret can't be ruled
// out." It is empty when nothing was unverified.
func unverifiedLine(names []string) string {
	if len(names) == 0 {
		return ""
	}
	var consequences []string
	seen := map[string]bool{}
	for _, name := range names {
		kind, _, _ := strings.Cut(name, " in ")
		consequence, ok := unverifiedConsequences[kind]
		if !ok {
			consequence = "a problem with " + kind
		}
		if !seen[consequence] {
			seen[consequence] = true
			consequences = append(consequences, consequence)
		}
	}
	return "I can't see " + joinOr(names) + ", so " +
		joinOr(consequences) + " can't be ruled out."
}

// joinOr lists values as "a", "a or b", or "a, b or c".
func joinOr(values []string) string {
	if len(values) < 2 {
		return strings.Join(values, "")
	}
	last := len(values) - 1
	return strings.Join(values[:last], ", ") + " or " + values[last]
}
