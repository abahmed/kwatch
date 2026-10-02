package kube_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func serviceSlice(
	name, service string, ready ...bool,
) *discoveryv1.EndpointSlice {
	slice := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: testNamespace,
		Labels: map[string]string{discoveryv1.LabelServiceName: service},
	}}
	for _, up := range ready {
		slice.Endpoints = append(slice.Endpoints, discoveryv1.Endpoint{
			Conditions: discoveryv1.EndpointConditions{Ready: &up},
		})
	}
	return slice
}

func TestEndpointReaderCountsReadyEndpointsOfOneService(t *testing.T) {
	client := fake.NewSimpleClientset(
		serviceSlice("hook-a", "hook", true, false),
		serviceSlice("hook-b", "hook", true),
		serviceSlice("other", "other", true),
	)
	r := kube.EndpointReader{Client: client}
	service := inventory.CoreID(kube.KindService, testNamespace, "hook")

	ready, ok := r.ReadyEndpoints(context.Background(), service)

	assert.True(t, ok)
	assert.Equal(t, 2, ready)
}

func TestEndpointReaderReportsUnknownOnFailure(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "endpointslices",
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("forbidden")
		})
	service := inventory.CoreID(kube.KindService, testNamespace, "hook")

	_, ok := kube.EndpointReader{Client: client}.ReadyEndpoints(
		context.Background(), service)

	assert.False(t, ok)
	_, ok = kube.EndpointReader{}.ReadyEndpoints(context.Background(), service)
	assert.False(t, ok)
	pod := inventory.CoreID(kube.KindPod, testNamespace, "hook")
	_, ok = kube.EndpointReader{Client: client}.ReadyEndpoints(
		context.Background(), pod)
	assert.False(t, ok)
}

func TestLogReaderLinesReadsFakeLogs(t *testing.T) {
	r := kube.LogReader{Client: fake.NewSimpleClientset(pod("p"))}
	id := kube.ContainerID(testNamespace, "p", "app")

	assert.Equal(t, []string{"fake logs"}, r.Lines(context.Background(), id))
	assert.Nil(t, kube.LogReader{}.Lines(context.Background(), id))
}
