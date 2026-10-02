package kube

import (
	"sort"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// MaxDynamicStatusKinds caps DynamicStatus.Kinds, so a cluster with
// thousands of custom resource types cannot grow the health response.
const MaxDynamicStatusKinds = 50

// DynamicKind is one dynamic resource type that is skipped, unavailable
// or capped, named "resource.group" (or "resource" for the core group).
type DynamicKind struct {
	Resource string
	Reason   string
}

func resourceName(gvr schema.GroupVersionResource) string {
	if gvr.Group == "" {
		return gvr.Resource
	}
	return gvr.Resource + "." + gvr.Group
}

// boundDynamicKinds sorts kinds by name and keeps the first
// MaxDynamicStatusKinds, reporting whether any were dropped.
func boundDynamicKinds(kinds []DynamicKind) ([]DynamicKind, bool) {
	sort.Slice(kinds, func(i, j int) bool {
		if kinds[i].Resource != kinds[j].Resource {
			return kinds[i].Resource < kinds[j].Resource
		}
		return kinds[i].Reason < kinds[j].Reason
	})
	if len(kinds) > MaxDynamicStatusKinds {
		return kinds[:MaxDynamicStatusKinds], true
	}
	return kinds, false
}
