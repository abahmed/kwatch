package rootcause

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// kindPlurals names the kinds kwatch may be unable to see in plain
// words. Other kinds take their lower-case name in the plural; see
// pluralOf.
var kindPlurals = map[inventory.Kind]string{
	kube.KindSecret:        "secrets",
	kube.KindConfigMap:     "config maps",
	kube.KindAccount:       "service accounts",
	kube.KindPVC:           "volume claims",
	kube.KindNetworkPolicy: "network policies",
	kube.KindNode:          "nodes",
}

// UnverifiedScope names the objects of id's kind and namespace in plain
// words, such as "secrets in billing" or "nodes".
func UnverifiedScope(id inventory.EntityID) string {
	return unverifiedKind(id.Kind, id.Namespace)
}

func unverifiedKind(kind inventory.Kind, namespace string) string {
	name, ok := kindPlurals[kind]
	if !ok {
		name = pluralOf(string(kind))
	}
	if namespace == "" {
		return name
	}
	return name + " in " + namespace
}

// pluralOf makes the plural of a lower-case kind name: "ingress"
// becomes "ingresses", "registry" becomes "registries" and the rest
// take an "s".
func pluralOf(name string) string {
	switch {
	case strings.HasSuffix(name, "s"), strings.HasSuffix(name, "x"),
		strings.HasSuffix(name, "z"), strings.HasSuffix(name, "ch"),
		strings.HasSuffix(name, "sh"):
		return name + "es"
	case strings.HasSuffix(name, "y") && len(name) > 1 &&
		!strings.ContainsAny(name[len(name)-2:len(name)-1], "aeiou"):
		return name[:len(name)-1] + "ies"
	}
	return name + "s"
}

// MergeUnverified returns the names sorted and without duplicates, or nil.
func MergeUnverified(names ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range names {
		for _, name := range list {
			if name != "" && !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out
}
