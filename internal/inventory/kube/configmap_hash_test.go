package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testConfigMap(value string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cm", Namespace: "default",
			Annotations: map[string]string{
				corev1.LastAppliedConfigAnnotation: "password: " + value,
			},
		},
		Data:       map[string]string{"app.conf": value},
		BinaryData: map[string][]byte{"blob": []byte(value)},
	}
}

func TestTransformHashesConfigMapValues(t *testing.T) {
	out, err := transform(testConfigMap("hunter2"))
	require.NoError(t, err)
	cm := out.(*corev1.ConfigMap)
	assert.NotContains(t, cm.Data["app.conf"], "hunter2")
	assert.NotEmpty(t, cm.Data["app.conf"])
	assert.NotContains(t, string(cm.BinaryData["blob"]), "hunter2")
	assert.NotContains(t, cm.Annotations,
		corev1.LastAppliedConfigAnnotation)
}

func TestTransformConfigMapKeepsChangeDetection(t *testing.T) {
	tt := []struct {
		name       string
		before     string
		after      string
		wantChange bool
	}{
		{"same_value_is_no_change", "v1", "v1", false},
		{"new_value_is_a_change", "v1", "v2", true},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := transform(testConfigMap(tc.before))
			after, _ := transform(testConfigMap(tc.after))
			changes := ConfigMapSchema{}.Diff(before, after)
			assert.Equal(t, tc.wantChange, len(changes) > 0)
			b, _ := ConfigMapSchema{}.Describe(before)
			a, _ := ConfigMapSchema{}.Describe(after)
			sameDigest := b.Attributes[AttrDataDigest].AsText() ==
				a.Attributes[AttrDataDigest].AsText()
			assert.Equal(t, !tc.wantChange, sameDigest)
		})
	}
}

func TestHashConfigMapDataIgnoresOtherObjects(t *testing.T) {
	pod := &corev1.Pod{}
	out, err := NewDigester(nil).HashConfigMapData(pod)
	require.NoError(t, err)
	assert.Same(t, pod, out)
}

func TestDigesterIsKeyedAndStable(t *testing.T) {
	key := []byte("install-key-generated-once")
	first, second := NewDigester(key), NewDigester(key)
	other := NewDigester([]byte("another-install"))

	value := []byte("hunter2")
	assert.Equal(t, first.digest(value), second.digest(value),
		"a stored key gives the same digest after a restart")
	assert.NotEqual(t, first.digest(value), other.digest(value))
	assert.NotEqual(t, digest(value), first.digest(value),
		"secret digests must not be the unkeyed SHA-256")
	assert.Len(t, first.digest(value), 2*digestBytes)
}

func TestDigesterEmptyKeyIsRandomPerInstance(t *testing.T) {
	value := []byte("hunter2")
	assert.NotEqual(t, NewDigester(nil).digest(value),
		NewDigester(nil).digest(value))
}

func TestTransformHashesSecretWithKey(t *testing.T) {
	secret := &corev1.Secret{Data: map[string][]byte{"pw": []byte("x")}}
	d := NewDigester([]byte("k"))
	out, err := d.HashSecretData(secret)
	require.NoError(t, err)
	assert.Equal(t, d.digest([]byte("x")),
		string(out.(*corev1.Secret).Data["pw"]))
}

func TestKeyedSchemasDetectValueChanges(t *testing.T) {
	d := NewDigester([]byte("k"))
	schema := NewConfigMapSchema(d)
	before, _ := d.HashConfigMapData(testConfigMap("v1"))
	after, _ := d.HashConfigMapData(testConfigMap("v2"))
	assert.NotEmpty(t, schema.Diff(before, after))

	secrets := NewSecretSchema(d)
	s1 := &corev1.Secret{Data: map[string][]byte{"pw": []byte("a")}}
	s2 := &corev1.Secret{Data: map[string][]byte{"pw": []byte("b")}}
	h1, _ := d.HashSecretData(s1)
	h2, _ := d.HashSecretData(s2)
	a, _ := secrets.Describe(h1)
	b, _ := secrets.Describe(h2)
	assert.NotEqual(t, a.Attributes[AttrDataDigest].AsText(),
		b.Attributes[AttrDataDigest].AsText())
	assert.NotEmpty(t, secrets.Diff(h1, h2))
}
