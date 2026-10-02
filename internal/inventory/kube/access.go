package kube

// Resource names an API resource by group and plural name.
type Resource struct {
	Group string
	Name  string
}

// Access is one permission the Kubernetes sources use. Required access
// feeds the typed informers readiness waits for (see requiredResources);
// optional access only widens detection or enriches analysis, and missing
// it degrades the detectors and rules that use it.
type Access struct {
	Resource Resource
	// Verb is the API verb, such as list or get.
	Verb string
	// Namespace restricts the check to one namespace; empty means all.
	Namespace string
	// Name restricts the check to one object, for grants scoped with
	// resourceNames such as the leader-election Lease.
	Name string
	// NonResourceURL is set for non-resource endpoints such as /readyz.
	NonResourceURL string
	Required       bool
}

// SourceAccess lists every permission the sources use. It is derived from
// the informer registrations, so adding a watched kind updates the RBAC
// audit without a second list to maintain.
func SourceAccess() []Access {
	var out []Access
	seen := map[Resource]bool{}
	watch := func(r Resource, required bool) {
		if seen[r] {
			return
		}
		seen[r] = true
		for _, verb := range []string{"list", "watch"} {
			out = append(out, Access{Resource: r, Verb: verb,
				Required: required})
		}
	}
	for _, r := range registrations() {
		watch(r.resource, r.required())
	}
	watch(eventsResource, false)
	for _, r := range auditedDynamicResources() {
		watch(r, false)
	}
	out = append(out,
		// Kubelet stats and metrics are read from each kubelet directly;
		// the kubelet authorizes them as get on nodes/stats and
		// nodes/metrics. nodes/proxy is not needed.
		Access{Resource: Resource{Name: "nodes/stats"}, Verb: "get"},
		Access{Resource: Resource{Name: "nodes/metrics"}, Verb: "get"},
		// Investigation reads a short log excerpt for announcements.
		Access{Resource: Resource{Name: "pods/log"}, Verb: "get"},
		Access{NonResourceURL: "/readyz", Verb: "get"},
		Access{Resource: Resource{Group: "coordination.k8s.io",
			Name: "leases"}, Verb: "get", Namespace: "kube-system"},
		// Restart evidence reads the node the previous kwatch Pod ran on.
		Access{Resource: Resource{Name: "nodes"}, Verb: "get"},
	)
	return out
}

// WithoutResource drops every check on resource, such as Secrets when
// watch.secrets is false, so the audit does not expect access kwatch was
// deliberately not granted.
func WithoutResource(checks []Access, resource Resource) []Access {
	out := make([]Access, 0, len(checks))
	for _, check := range checks {
		if check.Resource != resource || check.NonResourceURL != "" {
			out = append(out, check)
		}
	}
	return out
}

// InstallAccess lists the permissions kwatch uses inside its own
// namespace beyond the Lease it holds. Restart evidence reads the
// previous kwatch Pod by name, so get on pods is granted there only.
func InstallAccess(namespace string) []Access {
	return []Access{{Resource: Resource{Name: "pods"}, Verb: "get",
		Namespace: namespace}}
}

// TypedWatched counts the typed informers by watch mode, leaving out the
// resources listed as unavailable. The typed set is fixed, so the result
// is bounded.
func TypedWatched(unavailable []SourceStatus) map[WatchMode]int {
	down := map[Resource]bool{}
	for _, s := range unavailable {
		down[Resource{Group: s.Group, Name: s.Resource}] = true
	}
	out := map[WatchMode]int{}
	for _, r := range registrations() {
		if !down[r.resource] {
			out[r.mode()]++
		}
	}
	return out
}
