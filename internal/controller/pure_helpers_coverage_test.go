package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func TestNodeSelectorHelpersSupportOperatorsAndRejectUnknown(t *testing.T) {
	for _, operator := range []corev1.NodeSelectorOperator{
		corev1.NodeSelectorOpIn, corev1.NodeSelectorOpNotIn,
		corev1.NodeSelectorOpExists, corev1.NodeSelectorOpDoesNotExist,
		corev1.NodeSelectorOpGt, corev1.NodeSelectorOpLt,
	} {
		values := []string{"east"}
		if operator == corev1.NodeSelectorOpExists ||
			operator == corev1.NodeSelectorOpDoesNotExist {
			values = nil
		}
		if operator == corev1.NodeSelectorOpGt ||
			operator == corev1.NodeSelectorOpLt {
			values = []string{"1"}
		}
		selector, err := selectorFromNodeSelectorTerm(corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key: "zone", Operator: operator, Values: values,
			}},
		})
		if err != nil || selector == nil {
			t.Fatalf("selector for %s = %v", operator, err)
		}
	}
	if _, err := toSelectionOperator("unknown"); err == nil {
		t.Fatal("unknown selector operator was accepted")
	}
	if _, err := selectorFromNodeSelectorTerm(corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{{
			Key: "", Operator: corev1.NodeSelectorOpIn,
		}},
	}); err == nil {
		t.Fatal("invalid selector requirement was accepted")
	}
}

func TestEndpointHelpersClassifyTrafficAndObjects(t *testing.T) {
	falseValue := false
	trueValue := true
	tests := []struct {
		name       string
		conditions discoveryv1.EndpointConditions
		want       bool
	}{
		{name: "terminating", conditions: discoveryv1.EndpointConditions{
			Terminating: &trueValue, Ready: &trueValue,
		}},
		{name: "ready", conditions: discoveryv1.EndpointConditions{
			Ready: &trueValue,
		}, want: true},
		{name: "not ready", conditions: discoveryv1.EndpointConditions{
			Ready: &falseValue,
		}},
		{name: "serving", conditions: discoveryv1.EndpointConditions{
			Serving: &trueValue,
		}, want: true},
		{name: "not serving", conditions: discoveryv1.EndpointConditions{
			Serving: &falseValue,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := endpointCanReceiveTraffic(
				discoveryv1.Endpoint{Conditions: tt.conditions},
			); got != tt.want {
				t.Fatalf("traffic = %t, want %t", got, tt.want)
			}
		})
	}
	if got := endpointAddress(discoveryv1.Endpoint{}); got != "unknown" {
		t.Fatalf("empty endpoint address = %q", got)
	}
	if got := endpointAddress(discoveryv1.Endpoint{
		Addresses: []string{"10.0.0.1"},
	}); got != "10.0.0.1" {
		t.Fatalf("endpoint address = %q", got)
	}
}

func TestControllerGraphAndEndpointKeys(t *testing.T) {
	if !isTrackedWorkload("deployment") || isTrackedWorkload("service") {
		t.Fatal("tracked workload classification is wrong")
	}
	targets := ownedByTargets("apps", []metav1.OwnerReference{
		{Kind: "Deployment", Name: "api"},
		{Kind: "Service", Name: "ignored"},
	})
	if len(targets) != 1 || targets[0].Kind != "deployment" {
		t.Fatalf("owner targets = %+v", targets)
	}
	ep := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{
		Namespace: "apps", Labels: map[string]string{
			endpointSliceServiceLabel: "api",
		},
	}}
	deleted := cache.DeletedFinalStateUnknown{Obj: ep}
	keys := endpointSliceServiceKeys(ep, deleted, "invalid")
	if len(keys) != 1 || endpointSliceServiceKey(ep) != "apps/api" {
		t.Fatalf("endpoint service keys = %v", keys)
	}
	if endpointSliceObject(nil) != nil || endpointSliceServiceKey(
		&corev1.Pod{},
	) != "" {
		t.Fatal("invalid endpoint object was accepted")
	}
}
