package kube

import "strings"

// roleWords end the names of the parts an operator installs; the stem
// before them names the operator.
var roleWords = []string{
	"-webhook-service", "-webhook-server", "-webhook", "-controller",
	"-operator", "-manager", "-service", "-svc", "-server",
}

// minStem is the shortest stem that names an operator: a shorter one
// ("my", "app") is shared by unrelated objects.
const minStem = 4

// NamedAfter reports a name that starts with owner, or with owner
// without its role word, followed by a separator: "policy-tls" and
// "policy-controller-cert" are named after "policy-controller". An
// operator names what it installs after itself, so the objects one
// installation left behind find each other by name.
func NamedAfter(name, owner string) bool {
	stems := []string{owner}
	for _, word := range roleWords {
		if stem, ok := strings.CutSuffix(owner, word); ok {
			stems = append(stems, stem)
		}
	}
	for _, stem := range stems {
		if len(stem) < minStem {
			continue
		}
		if rest, ok := strings.CutPrefix(name, stem); ok &&
			(rest == "" || rest[0] == '-' || rest[0] == '.') {
			return true
		}
	}
	return false
}
