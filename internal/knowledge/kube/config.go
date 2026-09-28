package kube

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// Attribute names for configuration entities.
const (
	AttrSecretType = "secret.type"
	AttrKeys       = "keys"
	AttrCertExpiry = "tls.not.after"
)

// certExpiryAnnotation carries a TLS certificate's expiry on the cached
// Secret after HashSecretData replaced the certificate itself.
const certExpiryAnnotation = "kwatch.dev/tls-not-after"

// HashSecretData is an informer transform for Secrets. It replaces every
// value with a short digest, so the cache never holds secret material but
// data changes remain detectable, and records the TLS certificate expiry.
func HashSecretData(obj any) (any, error) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return obj, nil
	}
	cert := secret.Data[corev1.TLSCertKey]
	if notAfter, ok := certificateExpiry(cert); ok {
		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}
		secret.Annotations[certExpiryAnnotation] =
			notAfter.Format(time.RFC3339)
	}
	for key, value := range secret.Data {
		secret.Data[key] = []byte(digest(value))
	}
	secret.StringData = nil
	delete(secret.Annotations, corev1.LastAppliedConfigAnnotation)
	return secret, nil
}

// SecretSchema describes Secrets by key names and value digests.
type SecretSchema struct{}

// Kind implements Schema.
func (SecretSchema) Kind() knowledge.Kind { return KindSecret }

// RelationTypes implements Schema.
func (SecretSchema) RelationTypes() []knowledge.RelationType { return nil }

// Describe implements Schema.
func (SecretSchema) Describe(obj any) (Description, bool) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return Description{}, false
	}
	attrs := map[string]knowledge.Value{
		AttrSecretType: knowledge.Text(string(secret.Type)),
		AttrKeys: knowledge.Text(
			strings.Join(byteKeys(secret.Data), ",")),
	}
	if raw := secret.Annotations[certExpiryAnnotation]; raw != "" {
		if notAfter, err := time.Parse(time.RFC3339, raw); err == nil {
			attrs[AttrCertExpiry] = knowledge.Time(notAfter)
		}
	}
	return Description{
		ID: objectID(KindSecret, secret), UID: string(secret.UID),
		Attributes: attrs,
	}, true
}

// Diff implements Schema: which keys were added, removed or changed. Only
// key names are reported; values are never exposed.
func (SecretSchema) Diff(old, new any) []knowledge.FieldChange {
	before, ok1 := old.(*corev1.Secret)
	after, ok2 := new.(*corev1.Secret)
	if !ok1 || !ok2 {
		return nil
	}
	return keyChanges(digests(before.Data), digests(after.Data))
}

// ConfigMapSchema describes ConfigMaps by key names and value digests.
type ConfigMapSchema struct{}

// Kind implements Schema.
func (ConfigMapSchema) Kind() knowledge.Kind { return KindConfigMap }

// RelationTypes implements Schema.
func (ConfigMapSchema) RelationTypes() []knowledge.RelationType { return nil }

// Describe implements Schema.
func (ConfigMapSchema) Describe(obj any) (Description, bool) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return Description{}, false
	}
	keys := make([]string, 0, len(cm.Data)+len(cm.BinaryData))
	for key := range configMapDigests(cm) {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return Description{
		ID: objectID(KindConfigMap, cm), UID: string(cm.UID),
		Attributes: map[string]knowledge.Value{
			AttrKeys: knowledge.Text(strings.Join(keys, ",")),
		},
	}, true
}

// Diff implements Schema.
func (ConfigMapSchema) Diff(old, new any) []knowledge.FieldChange {
	before, ok1 := old.(*corev1.ConfigMap)
	after, ok2 := new.(*corev1.ConfigMap)
	if !ok1 || !ok2 {
		return nil
	}
	return keyChanges(configMapDigests(before), configMapDigests(after))
}

// ServiceAccountSchema describes ServiceAccounts.
type ServiceAccountSchema struct{}

// Kind implements Schema.
func (ServiceAccountSchema) Kind() knowledge.Kind { return KindAccount }

// RelationTypes implements Schema.
func (ServiceAccountSchema) RelationTypes() []knowledge.RelationType {
	return []knowledge.RelationType{knowledge.References}
}

// Describe implements Schema.
func (ServiceAccountSchema) Describe(obj any) (Description, bool) {
	account, ok := obj.(*corev1.ServiceAccount)
	if !ok {
		return Description{}, false
	}
	rel := relations{}
	for _, ref := range account.ImagePullSecrets {
		rel.add(knowledge.References,
			knowledge.NewEntityID(KindSecret, account.Namespace, ref.Name))
	}
	return Description{
		ID: objectID(KindAccount, account), UID: string(account.UID),
		Attributes: map[string]knowledge.Value{}, Relations: rel,
	}, true
}

// Diff implements Schema.
func (ServiceAccountSchema) Diff(_, _ any) []knowledge.FieldChange {
	return nil
}

func keyChanges(before, after map[string]string) []knowledge.FieldChange {
	var fields []knowledge.FieldChange
	for _, key := range sortedKeys(after) {
		switch previous, ok := before[key]; {
		case !ok:
			fields = append(fields, knowledge.FieldChange{
				Path: "data." + key, After: "added",
			})
		case previous != after[key]:
			fields = append(fields, knowledge.FieldChange{
				Path: "data." + key, After: "changed",
			})
		}
	}
	for _, key := range sortedKeys(before) {
		if _, ok := after[key]; !ok {
			fields = append(fields, knowledge.FieldChange{
				Path: "data." + key, After: "removed",
			})
		}
	}
	return fields
}

func digests(data map[string][]byte) map[string]string {
	out := make(map[string]string, len(data))
	for key, value := range data {
		out[key] = string(value)
	}
	return out
}

func configMapDigests(cm *corev1.ConfigMap) map[string]string {
	out := make(map[string]string, len(cm.Data)+len(cm.BinaryData))
	for key, value := range cm.Data {
		out[key] = digest([]byte(value))
	}
	for key, value := range cm.BinaryData {
		out[key] = digest(value)
	}
	return out
}

func byteKeys(data map[string][]byte) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:6])
}

func certificateExpiry(data []byte) (time.Time, bool) {
	block, _ := pem.Decode(data)
	if block == nil {
		return time.Time{}, false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, false
	}
	return cert.NotAfter, true
}
