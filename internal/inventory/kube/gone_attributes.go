package kube

import "github.com/abahmed/kwatch/internal/inventory"

// goneAttributes are the attributes a deletion hands over with the Gone
// observation, so they stay readable on the tombstone. Only a built-in
// RBAC binding hands any: who it bound is what the root-cause engine
// needs to tell a deleted binding of the failing account from one of
// another account, and a deleted object has no attributes left to read.
func goneAttributes(desc Description) map[string]inventory.Value {
	id := desc.ID
	if id.Group != "" ||
		(id.Kind != KindRoleBinding && id.Kind != KindClusterRoleBinding) {
		return nil
	}
	var out map[string]inventory.Value
	for _, name := range []string{AttrRBACRole, AttrRBACAccounts} {
		if value, ok := desc.Attributes[name]; ok {
			if out == nil {
				out = map[string]inventory.Value{}
			}
			out[name] = value
		}
	}
	return out
}
