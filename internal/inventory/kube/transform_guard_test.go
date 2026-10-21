package kube

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func poisonedSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "db", Namespace: "shop", UID: "u1",
			ResourceVersion: "7",
		},
		Data: map[string][]byte{"password": []byte("hunter2")},
	}
}

func panics(any) (any, error) { panic("malformed object") }

func TestGuardTransformStubsAnObjectThatPanics(t *testing.T) {
	got, err := guardTransform("secrets", panics)(poisonedSecret())

	require.NoError(t, err)
	stub, ok := got.(*cappedStub)
	require.True(t, ok, "want the identity stub, got %T", got)
	assert.Equal(t, "db", stub.Name)
	assert.Equal(t, "shop", stub.Namespace)
	assert.Equal(t, "7", stub.ResourceVersion)
	data, err := json.Marshal(stub)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "hunter2")
	assert.NotContains(t, string(data), "password",
		"the stub must hold nothing of a Secret's data")
}

func TestGuardTransformKeepsTheResultWithoutAPanic(t *testing.T) {
	secret := poisonedSecret()
	got, err := guardTransform("secrets",
		func(obj any) (any, error) { return obj, nil })(secret)

	require.NoError(t, err)
	assert.Same(t, secret, got)
}

func TestTranslatorsIgnoreAStubInsteadOfInventingChanges(t *testing.T) {
	translator := NewTranslator(PodSchema{})
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "shop", UID: "u1", ResourceVersion: "1"}}
	stub := newCappedStub(pod)

	assert.Empty(t, translator.Added(stub, false, now))
	assert.Empty(t, translator.Updated(pod, stub, now),
		"a stubbed update must not read as a change")
	assert.Empty(t, translator.Deleted(stub, now))
}

func TestTranslatorsTreatARecoveredStubAsFirstSight(t *testing.T) {
	translator := NewTranslator(PodSchema{})
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "api", Namespace: "shop", UID: "u1", ResourceVersion: "2"}}
	stub := newCappedStub(pod)
	pod.ResourceVersion = "3" // the next event of the same object

	observations := translator.Updated(stub, pod, now)

	for _, o := range observations {
		assert.NotEqual(t, "changed", o.Kind.String(),
			"coming back from a stub must not record a change")
	}
	assert.NotEmpty(t, observations, "the pod is observed again")
}
