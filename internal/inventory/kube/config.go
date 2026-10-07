package kube

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Attribute names for configuration entities.
const (
	AttrSecretType = "secret.type"
	AttrKeys       = "keys"
	AttrCertExpiry = "tls.not.after"
	// AttrDataDigest fingerprints all values, so a change is detectable
	// without keeping any value.
	AttrDataDigest = "data.digest"
)

// certExpiryAnnotation carries a TLS certificate's expiry on the cached
// Secret after HashSecretData replaced the certificate itself.
const certExpiryAnnotation = "kwatch.dev/tls-not-after"

// digestBytes is how much of the HMAC each keyed digest keeps.
const digestBytes = 8

// Digester fingerprints Secret and ConfigMap values with HMAC-SHA256 under
// a per-install key. Without a key, a short digest of a low-entropy value
// such as a password could be reversed by trying candidates.
//
// The zero Digester has no key and uses a plain short SHA-256. Schemas use
// it for the outer digest of values the informer transform already
// replaced with keyed digests; it keeps those digests deterministic.
type Digester struct {
	key []byte
}

// NewDigester returns a keyed digester. An empty key gets a random
// per-process key: changes are still detected while the process runs, but
// digests differ after a restart. Callers that compare digests across
// restarts must pass a key they generated once and stored.
func NewDigester(key []byte) Digester {
	if len(key) == 0 {
		key = make([]byte, sha256.Size)
		// crypto/rand.Read never fails; it aborts the process instead.
		_, _ = rand.Read(key)
	}
	return Digester{key: append([]byte(nil), key...)}
}

func (d Digester) digest(value []byte) string {
	if len(d.key) == 0 {
		return digest(value)
	}
	mac := hmac.New(sha256.New, d.key)
	mac.Write(value)
	return hex.EncodeToString(mac.Sum(nil)[:digestBytes])
}

// HashSecretData is an informer transform for Secrets. It replaces every
// value with a short digest, so the cache never holds secret material but
// data changes remain detectable, and records the TLS certificate expiry.
func (d Digester) HashSecretData(obj any) (any, error) {
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
		secret.Data[key] = []byte(d.digest(value))
	}
	secret.StringData = nil
	delete(secret.Annotations, corev1.LastAppliedConfigAnnotation)
	return secret, nil
}

// HashConfigMapData is an informer transform for ConfigMaps. Values can
// hold credentials people should not have put there, so the cache keeps
// only a digest per key; key changes stay detectable through the digests.
func (d Digester) HashConfigMapData(obj any) (any, error) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return obj, nil
	}
	for key, value := range cm.Data {
		cm.Data[key] = d.digest([]byte(value))
	}
	for key, value := range cm.BinaryData {
		cm.BinaryData[key] = []byte(d.digest(value))
	}
	delete(cm.Annotations, corev1.LastAppliedConfigAnnotation)
	return cm, nil
}

// SecretSchema describes Secrets by key names and value digests. The zero
// value expects values the informer transform already digested.
type SecretSchema struct {
	digester Digester
}

// NewSecretSchema returns a schema whose digests use d's key.
func NewSecretSchema(d Digester) SecretSchema {
	return SecretSchema{digester: d}
}

// Kind implements Schema.
func (SecretSchema) Kind() inventory.Kind { return KindSecret }

// RelationTypes implements Schema.
func (SecretSchema) RelationTypes() []inventory.RelationType { return nil }

// Describe implements Schema.
func (s SecretSchema) Describe(obj any) (Description, bool) {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return Description{}, false
	}
	attrs := map[string]inventory.Value{
		AttrSecretType: inventory.Text(string(secret.Type)),
		AttrKeys: inventory.Text(
			strings.Join(byteKeys(secret.Data), ",")),
		AttrDataDigest: inventory.Text(
			mapDigestWith(s.digester, digests(secret.Data))),
	}
	setLoginChanged(attrs, secret)
	if raw := secret.Annotations[certExpiryAnnotation]; raw != "" {
		if notAfter, err := time.Parse(time.RFC3339, raw); err == nil {
			attrs[AttrCertExpiry] = inventory.Time(notAfter)
		}
	}
	return Description{
		ID: objectID(KindSecret, secret), UID: string(secret.UID),
		Attributes: attrs,
	}, true
}

// Diff implements Schema: which keys were added, removed or changed. Only
// key names are reported; values are never exposed.
func (SecretSchema) Diff(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*corev1.Secret)
	after, ok2 := new.(*corev1.Secret)
	if !ok1 || !ok2 {
		return nil
	}
	return keyChanges(digests(before.Data), digests(after.Data))
}

// ConfigMapSchema describes ConfigMaps by key names and value digests. The
// zero value expects values the informer transform already digested.
type ConfigMapSchema struct {
	digester Digester
}

// NewConfigMapSchema returns a schema whose digests use d's key.
func NewConfigMapSchema(d Digester) ConfigMapSchema {
	return ConfigMapSchema{digester: d}
}

// Kind implements Schema.
func (ConfigMapSchema) Kind() inventory.Kind { return KindConfigMap }

// RelationTypes implements Schema.
func (ConfigMapSchema) RelationTypes() []inventory.RelationType { return nil }

// Describe implements Schema.
func (s ConfigMapSchema) Describe(obj any) (Description, bool) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return Description{}, false
	}
	values := configMapDigestsWith(s.digester, cm)
	return Description{
		ID: objectID(KindConfigMap, cm), UID: string(cm.UID),
		Attributes: map[string]inventory.Value{
			AttrKeys: inventory.Text(
				strings.Join(sortedKeys(values), ",")),
			AttrDataDigest: inventory.Text(mapDigestWith(s.digester, values)),
		},
	}, true
}

// Diff implements Schema.
func (s ConfigMapSchema) Diff(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*corev1.ConfigMap)
	after, ok2 := new.(*corev1.ConfigMap)
	if !ok1 || !ok2 {
		return nil
	}
	return keyChanges(configMapDigestsWith(s.digester, before),
		configMapDigestsWith(s.digester, after))
}

// ServiceAccountSchema describes ServiceAccounts.
type ServiceAccountSchema struct{}

// Kind implements Schema.
func (ServiceAccountSchema) Kind() inventory.Kind { return KindAccount }

// RelationTypes implements Schema.
func (ServiceAccountSchema) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{inventory.References}
}

// Describe implements Schema.
func (ServiceAccountSchema) Describe(obj any) (Description, bool) {
	account, ok := obj.(*corev1.ServiceAccount)
	if !ok {
		return Description{}, false
	}
	rel := relations{}
	for _, ref := range account.ImagePullSecrets {
		rel.add(inventory.References,
			inventory.CoreID(KindSecret, account.Namespace, ref.Name))
	}
	return Description{
		ID: objectID(KindAccount, account), UID: string(account.UID),
		Attributes: map[string]inventory.Value{}, Relations: rel,
	}, true
}

// Diff implements Schema.
func (ServiceAccountSchema) Diff(_, _ any) []inventory.FieldChange {
	return nil
}

func keyChanges(before, after map[string]string) []inventory.FieldChange {
	var fields []inventory.FieldChange
	for _, key := range sortedKeys(after) {
		switch previous, ok := before[key]; {
		case !ok:
			fields = append(fields, inventory.FieldChange{
				Path: "data." + key, After: "added",
			})
		case previous != after[key]:
			fields = append(fields, inventory.FieldChange{
				Path: "data." + key, After: "changed",
			})
		}
	}
	for _, key := range sortedKeys(before) {
		if _, ok := after[key]; !ok {
			fields = append(fields, inventory.FieldChange{
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

// configMapDigests digests already-transformed values without a key.
func configMapDigests(cm *corev1.ConfigMap) map[string]string {
	return configMapDigestsWith(Digester{}, cm)
}

func configMapDigestsWith(
	d Digester, cm *corev1.ConfigMap,
) map[string]string {
	out := make(map[string]string, len(cm.Data)+len(cm.BinaryData))
	for key, value := range cm.Data {
		out[key] = d.digest([]byte(value))
	}
	for key, value := range cm.BinaryData {
		out[key] = d.digest(value)
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

// digest is a short unkeyed SHA-256 fingerprint for non-secret data such
// as a NetworkPolicy spec, or for values that are already keyed digests.
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

// mapDigest fingerprints a key → value-digest map in key order.
func mapDigest(values map[string]string) string {
	return mapDigestWith(Digester{}, values)
}

// mapDigestWith is mapDigest under d's key.
func mapDigestWith(d Digester, values map[string]string) string {
	var b strings.Builder
	for _, key := range sortedKeys(values) {
		b.WriteString(key + "=" + values[key] + ";")
	}
	return d.digest([]byte(b.String()))
}
