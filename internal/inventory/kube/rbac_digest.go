package kube

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// rulesDigestField is where rbacTransform keeps the digest of the rules
// it drops.
const rulesDigestField = "rulesDigest"

// rulesDigest is a short digest of a role's rules: the one the transform
// kept, or one over the rules the object still has. "" without rules.
func rulesDigest(u *unstructured.Unstructured) string {
	if kept, ok := u.Object[rulesDigestField].(string); ok {
		return kept
	}
	rules, ok := u.Object["rules"]
	if !ok {
		return ""
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:6])
}
