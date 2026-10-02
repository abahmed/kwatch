package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// Two processes given the same stored key hash a value the same way, so
// a restart does not make every ConfigMap look changed.
func TestNewTransformSameKeyGivesSameDigests(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	first, err := newTransform(NewDigester(key))(testConfigMap("v1"))
	require.NoError(t, err)
	second, err := newTransform(NewDigester(key))(testConfigMap("v1"))
	require.NoError(t, err)
	other, err := newTransform(NewDigester([]byte("other")))(
		testConfigMap("v1"))
	require.NoError(t, err)

	got := first.(*corev1.ConfigMap).Data["app.conf"]
	assert.Equal(t, got, second.(*corev1.ConfigMap).Data["app.conf"])
	assert.NotEqual(t, got, other.(*corev1.ConfigMap).Data["app.conf"])
	assert.NotContains(t, got, "v1")
}

func TestNewTransformHashesSecretsWithKey(t *testing.T) {
	secret := &corev1.Secret{Data: map[string][]byte{"token": []byte("s3")}}
	key := []byte("0123456789abcdef0123456789abcdef")

	first, err := newTransform(NewDigester(key))(secret.DeepCopy())
	require.NoError(t, err)
	second, err := newTransform(NewDigester(key))(secret.DeepCopy())
	require.NoError(t, err)

	assert.Equal(t, first.(*corev1.Secret).Data, second.(*corev1.Secret).Data)
	assert.NotEqual(t, []byte("s3"), first.(*corev1.Secret).Data["token"])
}
