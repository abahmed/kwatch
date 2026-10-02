package kube_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestHashSecretData(t *testing.T) {
	secret := secret("s1")
	secret.Data = map[string][]byte{
		"user": []byte("admin"),
		"pass": []byte("secret123"),
	}

	result, err := kube.NewDigester(nil).HashSecretData(secret)
	assert.NoError(t, err)

	s := result.(*corev1.Secret)
	// Data should be replaced with digests
	assert.NotEqual(t, "admin", string(s.Data["user"]))
	assert.NotEqual(t, "secret123", string(s.Data["pass"]))
	// Digests should be short
	assert.True(t, len(s.Data["user"]) < 32)
}

func TestHashSecretDataRemovesLastApplied(t *testing.T) {
	secret := secret("s1")
	secret.Annotations = map[string]string{
		corev1.LastAppliedConfigAnnotation: "config",
		"custom":                           "value",
	}

	result, err := kube.NewDigester(nil).HashSecretData(secret)
	assert.NoError(t, err)

	s := result.(*corev1.Secret)
	_, hasLastApplied := s.Annotations[corev1.LastAppliedConfigAnnotation]
	assert.False(t, hasLastApplied)
	// Custom annotation should remain
	assert.Equal(t, "value", s.Annotations["custom"])
}

func TestHashSecretDataWithTLSCert(t *testing.T) {
	secret := secret("s1")
	secret.Type = corev1.SecretTypeTLS

	// Create a test certificate
	cert := generateTestCert()
	secret.Data[corev1.TLSCertKey] = cert

	result, err := kube.NewDigester(nil).HashSecretData(secret)
	assert.NoError(t, err)

	s := result.(*corev1.Secret)
	// Certificate expiry should be in annotations
	_, hasExpiry := s.Annotations["kwatch.dev/tls-not-after"]
	assert.True(t, hasExpiry,
		"tls-not-after annotation not found")
}

func TestSecretSchemaDescribe(t *testing.T) {
	secret := secret("s1")
	secret.Type = corev1.SecretTypeOpaque
	secret.Data = map[string][]byte{
		"key1": []byte("digest1"),
		"key2": []byte("digest2"),
	}

	schema := kube.SecretSchema{}
	desc, ok := schema.Describe(secret)

	assert.True(t, ok)
	assert.Equal(t, inventory.Kind("secret"), desc.ID.Kind)
	assert.Equal(t, "s1", desc.ID.Name)

	// Check type
	sType, ok := desc.Attributes["secret.type"]
	assert.True(t, ok)
	assert.Equal(t, "Opaque", sType.AsText())

	// Check keys
	keys, ok := desc.Attributes["keys"]
	assert.True(t, ok)
	text := keys.AsText()
	assert.Contains(t, text, "key1")
	assert.Contains(t, text, "key2")
}

func TestSecretKeysDiff(t *testing.T) {
	old := secret("s1")
	old.Data = map[string][]byte{
		"key1": []byte("digest1"),
	}

	new := secret("s1")
	new.Data = map[string][]byte{
		"key1": []byte("different_digest"),
		"key2": []byte("digest2"),
	}

	schema := kube.SecretSchema{}
	changes := schema.Diff(old, new)

	// Should have changes for key1 changed and key2 added
	assert.Len(t, changes, 2)

	findChange := func(path string) *inventory.FieldChange {
		for i := range changes {
			if changes[i].Path == path {
				return &changes[i]
			}
		}
		return nil
	}

	c1 := findChange("data.key1")
	assert.NotNil(t, c1)
	assert.Equal(t, "changed", c1.After)

	c2 := findChange("data.key2")
	assert.NotNil(t, c2)
	assert.Equal(t, "added", c2.After)
}

func TestConfigMapSchemaDescribe(t *testing.T) {
	cm := configMap("cm1")
	cm.Data = map[string]string{
		"app.conf": "config_data",
		"app.yaml": "yaml_data",
	}
	cm.BinaryData = map[string][]byte{
		"binary": []byte("binary_data"),
	}

	schema := kube.ConfigMapSchema{}
	desc, ok := schema.Describe(cm)

	assert.True(t, ok)
	assert.Equal(t, inventory.Kind("configmap"), desc.ID.Kind)

	// Check keys include both data and binary data
	keys, ok := desc.Attributes["keys"]
	assert.True(t, ok)
	text := keys.AsText()
	assert.Contains(t, text, "app.conf")
	assert.Contains(t, text, "app.yaml")
	assert.Contains(t, text, "binary")
}

func TestConfigMapBinaryDataDiff(t *testing.T) {
	old := configMap("cm1")
	old.Data = map[string]string{
		"text": "data",
	}
	old.BinaryData = map[string][]byte{
		"bin": []byte("binary_content"),
	}

	new := configMap("cm1")
	new.Data = map[string]string{
		"text": "data",
	}
	new.BinaryData = map[string][]byte{
		"bin":  []byte("new_binary_content"),
		"bin2": []byte("another_binary"),
	}

	schema := kube.ConfigMapSchema{}
	changes := schema.Diff(old, new)

	// Should detect bin changed and bin2 added
	found := make(map[string]bool)
	for _, c := range changes {
		found[c.Path] = true
	}

	assert.True(t, found["data.bin"],
		"bin change not found")
	assert.True(t, found["data.bin2"],
		"bin2 addition not found")
}

func TestServiceAccountSchemaDescribe(t *testing.T) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa1",
			Namespace: testNamespace,
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: "pull-secret"},
		},
	}

	schema := kube.ServiceAccountSchema{}
	desc, ok := schema.Describe(sa)

	assert.True(t, ok)
	assert.Equal(t, inventory.Kind("serviceaccount"),
		desc.ID.Kind)
	assert.Equal(t, "sa1", desc.ID.Name)
}

func TestServiceAccountReferencesSecrets(t *testing.T) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sa1",
			Namespace: testNamespace,
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: "pull1"},
			{Name: "pull2"},
		},
	}

	schema := kube.ServiceAccountSchema{}
	desc, ok := schema.Describe(sa)
	assert.True(t, ok)

	refs := desc.Relations[inventory.References]
	assert.Len(t, refs, 2)

	// Verify both secrets are referenced
	names := make(map[string]bool)
	for _, r := range refs {
		assert.Equal(t, inventory.Kind("secret"), r.Kind)
		names[r.Name] = true
	}
	assert.True(t, names["pull1"])
	assert.True(t, names["pull2"])
}

func TestServiceAccountNoDiff(t *testing.T) {
	sa1 := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sa1", Namespace: testNamespace,
		},
	}
	sa2 := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sa1", Namespace: testNamespace,
		},
	}

	schema := kube.ServiceAccountSchema{}
	changes := schema.Diff(sa1, sa2)

	// ServiceAccounts have no diffs
	assert.Nil(t, changes)
}

// generateTestCert creates a self-signed certificate for testing.
func generateTestCert() []byte {
	// Generate RSA key
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment,
		PublicKey:    &privateKey.PublicKey,
		Subject: pkix.Name{
			CommonName: "test-cert",
		},
	}

	// Sign certificate
	certBytes, _ := x509.CreateCertificate(
		rand.Reader, &template, &template,
		&privateKey.PublicKey, privateKey)

	// PEM encode
	pemBlock := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	}

	return pem.EncodeToMemory(pemBlock)
}
