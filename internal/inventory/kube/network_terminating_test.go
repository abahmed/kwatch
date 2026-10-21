package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	discoveryv1 "k8s.io/api/discovery/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestEndpointSliceSchemaCountsTerminatingEndpoints(t *testing.T) {
	yes, no := true, false
	slice := &discoveryv1.EndpointSlice{Endpoints: []discoveryv1.Endpoint{
		{Conditions: discoveryv1.EndpointConditions{Ready: &yes}},
		{Conditions: discoveryv1.EndpointConditions{
			Ready: &no, Terminating: &yes}},
		{Conditions: discoveryv1.EndpointConditions{Ready: &no}},
	}}
	slice.Name, slice.Namespace = "api-1", "shop"

	desc, ok := kube.EndpointSliceSchema{}.Describe(slice)

	assert.True(t, ok)
	count := func(attribute string) float64 {
		n, _ := desc.Attributes[attribute].AsNumber()
		return n
	}
	assert.Equal(t, 3.0, count(kube.AttrEndpoints))
	assert.Equal(t, 1.0, count(kube.AttrEndpointsReady))
	assert.Equal(t, 1.0, count(kube.AttrEndpointsTerminating))
}

// A slice with no pod shutting down is described without the attribute,
// so the logs of clusters that never terminate a backend stay as they are.
func TestEndpointSliceSchemaOmitsTerminatingWhenNoneAre(t *testing.T) {
	yes := true
	slice := &discoveryv1.EndpointSlice{Endpoints: []discoveryv1.Endpoint{
		{Conditions: discoveryv1.EndpointConditions{Ready: &yes}},
	}}

	desc, ok := kube.EndpointSliceSchema{}.Describe(slice)

	assert.True(t, ok)
	_, present := desc.Attributes[kube.AttrEndpointsTerminating]
	assert.False(t, present)
}
