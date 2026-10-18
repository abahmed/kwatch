package crd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// configFingerprint identifies the configuration a KwatchConfig carries.
// It hashes the spec, so a re-applied identical manifest, which can still
// move resourceVersion through annotations, managedFields or labels, is
// not a change. An object without a readable spec falls back to its
// resourceVersion.
func configFingerprint(obj interface{}) string {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		if spec, found := u.Object["spec"]; found {
			if raw, err := json.Marshal(spec); err == nil {
				sum := sha256.Sum256(raw)
				return "spec:" + hex.EncodeToString(sum[:])
			}
		}
	}
	if accessor, err := meta.Accessor(obj); err == nil {
		return accessor.GetResourceVersion()
	}
	return ""
}
