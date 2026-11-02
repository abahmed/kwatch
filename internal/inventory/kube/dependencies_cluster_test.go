package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestServiceCallInReadsClusterNames(t *testing.T) {
	cases := []struct {
		name, value string
		want        string
	}{
		{"CACHE", "redis:6379", "shop/redis:6379"},
		{"CACHE", "redis.data:6379", "data/redis:6379"},
		{"CACHE", "redis.data.svc:6379", "data/redis:6379"},
		{"CACHE", "redis.data.svc.cluster.local:6379",
			"data/redis:6379"},
		{"DATABASE_URL", "postgres://u:p@db:5432/x", "shop/db:5432"},
		{"API_URL", "http://api.shop.svc.cluster.local", "shop/api:80"},
		{"REDIS_HOST", "redis", "shop/redis"},
		{"MODE", "production", ""},
		{"CACHE", "10.0.0.5:6379", ""},
		{"CACHE", "localhost:6379", ""},
		{"CACHE", "a:1,b:2", ""},
		{"CACHE", "", ""},
		// The pod's own name for its telemetry, not a call.
		{"OTEL_SERVICE_NAME", "api", ""},
		{"DD_SERVICE", "api", ""},
		{"SERVICE_NAME", "api", ""},
		// Words Kubernetes refuses as a Service or namespace name.
		{"API_HOST", "owner=api", ""},
		{"API_HOST", "my_api", ""},
		{"API_HOST", "api.bad_ns", ""},
	}
	for _, c := range cases {
		call, ok := serviceCallIn(c.name, c.value, "shop")
		assert.Equal(t, c.want != "", ok, c.value)
		if ok {
			assert.Equal(t, c.want, call.String(), c.value)
		}
	}
}

func TestPodServiceCallsKeepsServicesAndAttribute(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "app", Env: []corev1.EnvVar{
				{Name: "CACHE", Value: "redis:6379"},
				{Name: "DB", Value: "db.data.svc:5432"},
				{Name: "EXT", Value: "db.example.com:5432"},
			}}}},
	}

	desc, ok := PodSchema{}.Describe(pod)

	assert.True(t, ok)
	assert.Equal(t, "data/db:5432,shop/redis:6379",
		desc.Attributes[AttrServiceCalls].AsText())
	calls := desc.Relations[inventory.Calls]
	assert.Contains(t, calls, inventory.CoreID(KindService, "shop", "redis"))
	assert.Contains(t, calls, inventory.CoreID(KindService, "data", "db"))
	assert.Contains(t, calls, inventory.CoreID(KindExternalEndpoint, "",
		"db.example.com:5432"))
	refs := ServiceCalls(inventory.Entity{Attributes: map[string]inventory.
		Attribute{AttrServiceCalls: {Value: desc.Attributes[AttrServiceCalls]}}})
	assert.Equal(t, 5432, refs[0].Port)
}

func TestServiceCallsSkipsInvalidServiceNames(t *testing.T) {
	pod := inventory.Entity{Attributes: map[string]inventory.Attribute{
		AttrServiceCalls: {Value: inventory.Text(
			"shop/owner=api,shop/redis:6379,bad_ns/db")}}}

	refs := ServiceCalls(pod)

	assert.Equal(t, []ServiceRef{{Service: inventory.CoreID(KindService,
		"shop", "redis"), Port: 6379}}, refs)
}

func TestPodServiceCallsSkipsCredentialNamedVariables(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Env: []corev1.EnvVar{
				{Name: "CACHE_PASSWORD_HOST", Value: "redis:6379"},
				{Name: "API_TOKEN_URL", Value: "http://api.shop.svc:8080"},
				{Name: "DB_HOST", Value: "db:5432"},
			},
		}}},
	}

	calls := podServiceCalls(pod)

	if assert.Len(t, calls, 1) {
		assert.Equal(t, "shop/db:5432", calls[0].String())
	}
}
