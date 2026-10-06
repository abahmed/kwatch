package kube

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestDependencyInReadsHostAndPortOnly(t *testing.T) {
	cases := map[string]string{
		"postgres://app:s3cret@db.example.com:5432/orders?sslmode=require": "" +
			"db.example.com:5432",
		"postgres://db.example.com/orders":            "db.example.com:5432",
		"redis://cache.example.com":                   "cache.example.com:6379",
		"https://api.stripe.com/v1":                   "api.stripe.com:443",
		"Kafka.Example.COM:9092":                      "kafka.example.com:9092",
		"orders-db.internal.corp:5432":                "orders-db.internal.corp:5432",
		"http://payments.shop.svc.cluster.local:8080": "",
		"redis://cache.shop.svc:6379":                 "",
		"http://localhost:8080":                       "",
		"10.0.5.7:5432":                               "",
		"postgres://db:5432/orders":                   "",
		"debug":                                       "",
		"8080":                                        "",
		"":                                            "",
	}
	for value, want := range cases {
		got, ok := dependencyIn(value)
		assert.Equal(t, want != "", ok, value)
		assert.Equal(t, want, got, value)
	}
}

func TestPodDependenciesListsEachEndpointOnce(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{
		{Name: "app", Env: []corev1.EnvVar{
			{Name: "DATABASE_URL", Value: "postgres://db.example.com/x"},
			{Name: "CACHE", Value: "cache.example.com:6379"},
			{Name: "FROM_SECRET", ValueFrom: &corev1.EnvVarSource{}},
		}},
		{Name: "sidecar", Env: []corev1.EnvVar{
			{Name: "DB", Value: "postgres://db.example.com:5432/y"},
		}},
	}}}

	got := podDependencies(pod)

	names := make([]string, 0, len(got))
	for _, id := range got {
		assert.Equal(t, KindExternalEndpoint, id.Kind)
		names = append(names, id.Name)
	}
	assert.Equal(t, []string{"cache.example.com:6379", "db.example.com:5432"},
		names)
}

func TestPodDependenciesAreCappedPerPod(t *testing.T) {
	var env []corev1.EnvVar
	for i := range 3 * maxPodDependencies {
		env = append(env, corev1.EnvVar{
			Name:  fmt.Sprintf("DB_%d", i),
			Value: fmt.Sprintf("db%03d.example.com:5432", i),
		})
	}
	pod := &corev1.Pod{Spec: corev1.PodSpec{
		Containers: []corev1.Container{{Name: "app", Env: env}}}}

	got := podDependencies(pod)

	require.Len(t, got, maxPodDependencies)
	assert.Equal(t, "db000.example.com:5432", got[0].Name,
		"the first endpoints by name are kept")
}
