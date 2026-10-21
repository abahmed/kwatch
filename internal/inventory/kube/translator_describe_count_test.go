package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// countingSchema counts Describe calls.
type countingSchema struct {
	Schema
	describes int
}

func (c *countingSchema) Describe(obj any) (Description, bool) {
	c.describes++
	return c.Schema.Describe(obj)
}

func TestTranslatorUpdatedDescribesEachObjectOnce(t *testing.T) {
	schema := &countingSchema{Schema: PodSchema{}}
	translator := NewTranslator(schema)
	pod := func(version string) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name: "api", Namespace: "shop", UID: "u1",
			ResourceVersion: version}}
	}

	observations := translator.Updated(pod("1"), pod("2"),
		time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))

	assert.NotEmpty(t, observations)
	assert.Equal(t, 2, schema.describes,
		"one Describe for the old object and one for the new")
}
