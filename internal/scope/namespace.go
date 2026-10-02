package scope

import "github.com/abahmed/kwatch/internal/inventory"

// NamespaceAllowed reports whether namespace is inside the configured
// namespace scope: the allowed and forbidden lists and the namespace label
// selector. Reasons and silences are not considered. A nil Scope allows
// every namespace, and the empty (cluster-scoped) namespace is always in.
func (s *Scope) NamespaceAllowed(
	model inventory.Reader, namespace string,
) bool {
	if s == nil {
		return true
	}
	return s.namespaceAllowed(model, namespace)
}
