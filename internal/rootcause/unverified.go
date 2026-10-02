package rootcause

import (
	"sort"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// kindPlurals names the kinds kwatch may be unable to see in plain
// words. Other kinds
// take their lower-case name with an "s".
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
		name = string(kind) + "s"
	}
	if namespace == "" {
		return name
	}
	return name + " in " + namespace
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
