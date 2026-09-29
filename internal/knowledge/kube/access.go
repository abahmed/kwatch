package kube

// Resource names an API resource by group and plural name.
type Resource struct {
	Group string
	Name  string
}

// Access is one permission the Kubernetes sources use. Required access
// feeds the typed informers the core needs to be ready; optional access
// only enriches analysis, and missing it degrades the rules that use it.
type Access struct {
	Resource Resource
	// Verb is the API verb, such as list or get.
	Verb string
	// Namespace restricts the check to one namespace; empty means all.
	Namespace string
	// NonResourceURL is set for non-resource endpoints such as /readyz.
	NonResourceURL string
	Required       bool
}

// SourceAccess lists every permission the sources use. It is derived from
// the informer registrations, so adding a watched kind updates the RBAC
// audit without a second list to maintain.
func SourceAccess() []Access {
	var out []Access
	watch := func(r Resource, required bool) {
		for _, verb := range []string{"list", "watch"} {
			out = append(out, Access{Resource: r, Verb: verb,
				Required: required})
		}
	}
	for _, r := range registrations() {
		watch(r.resource, true)
	}
	watch(Resource{Name: "events"}, true)
	for _, r := range builtinResources {
		watch(Resource{Group: r.gvr.Group, Name: r.gvr.Resource}, false)
	}
	watch(Resource{Group: apiServiceResource.Group,
		Name: apiServiceResource.Resource}, false)
	out = append(out,
		Access{Resource: Resource{Group: crdResource.Group,
			Name: crdResource.Resource}, Verb: "list"},
		// Kubelet stats and metrics are read through the API server proxy.
		Access{Resource: Resource{Name: "nodes/proxy"}, Verb: "get"},
		// Investigation reads a short log excerpt for announcements.
		Access{Resource: Resource{Name: "pods/log"}, Verb: "get"},
		Access{NonResourceURL: "/readyz", Verb: "get"},
		Access{Resource: Resource{Group: "coordination.k8s.io",
			Name: "leases"}, Verb: "get", Namespace: "kube-system"},
	)
	return out
}
