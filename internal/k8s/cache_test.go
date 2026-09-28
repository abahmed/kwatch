package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTrimManagedFieldsRemovesManagedFieldsMetadata(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{
					Manager: "kubelet",
				},
			},
		},
	}

	result, err := TrimManagedFields(pod)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.(*corev1.Pod).ObjectMeta.ManagedFields)
}

func TestTrimManagedFieldsNonSecretObject(t *testing.T) {
	configmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cm",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{
					Manager: "kubectl",
				},
			},
		},
		Data: map[string]string{"key": "value"},
	}

	result, err := TrimManagedFields(configmap)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.(*corev1.ConfigMap).ObjectMeta.ManagedFields)
	assert.Equal(t, "value", result.(*corev1.ConfigMap).Data["key"])
}

func TestTrimManagedFieldsSecretsRemovesDataAndStringData(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-secret",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"password": []byte("hunter2"),
			"api_key":  []byte("secret123"),
		},
		StringData: map[string]string{
			"token": "abc123",
		},
	}

	result, err := TrimManagedFields(secret)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	resultSecret := result.(*corev1.Secret)
	assert.Nil(t, resultSecret.Data)
	assert.Nil(t, resultSecret.StringData)
}

func TestTrimManagedFieldsTLSSecretKeepsOnlyClientCert(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tls-secret",
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       []byte("-----BEGIN CERTIFICATE-----"),
			corev1.TLSPrivateKeyKey: []byte("-----BEGIN PRIVATE KEY-----"),
		},
	}

	result, err := TrimManagedFields(secret)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	resultSecret := result.(*corev1.Secret)
	assert.NotNil(t, resultSecret.Data)
	assert.Len(t, resultSecret.Data, 1)
	assert.Contains(t, resultSecret.Data, corev1.TLSCertKey)
	assert.NotContains(t, resultSecret.Data, corev1.TLSPrivateKeyKey)
	assert.Equal(
		t,
		"-----BEGIN CERTIFICATE-----",
		string(resultSecret.Data[corev1.TLSCertKey]),
	)
}

func TestTrimManagedFieldsTLSSecretWithoutCertRemovesData(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "incomplete-tls",
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSPrivateKeyKey: []byte("-----BEGIN PRIVATE KEY-----"),
		},
	}

	result, err := TrimManagedFields(secret)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	resultSecret := result.(*corev1.Secret)
	assert.Nil(t, resultSecret.Data)
}

func TestTrimManagedFieldsRemovesLastAppliedAnnotation(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "annotated-secret",
			Annotations: map[string]string{
				corev1.LastAppliedConfigAnnotation: `{"data":"sensitive"}`,
				"other-annotation":                 "value",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"key": []byte("value"),
		},
	}

	result, err := TrimManagedFields(secret)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	resultSecret := result.(*corev1.Secret)
	assert.Nil(t, resultSecret.Data)
	assert.NotContains(
		t,
		resultSecret.ObjectMeta.Annotations,
		corev1.LastAppliedConfigAnnotation,
	)
	assert.Equal(
		t,
		"value",
		resultSecret.ObjectMeta.Annotations["other-annotation"],
	)
}

func TestTrimManagedFieldsNilDataSecret(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "empty-secret",
		},
		Type: corev1.SecretTypeOpaque,
	}

	result, err := TrimManagedFields(secret)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	resultSecret := result.(*corev1.Secret)
	assert.Nil(t, resultSecret.Data)
}
