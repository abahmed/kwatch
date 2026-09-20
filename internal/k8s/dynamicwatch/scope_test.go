package dynamicwatch

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	discoveryfake "k8s.io/client-go/discovery/fake"
	discoverytesting "k8s.io/client-go/testing"
)

func TestNamespaces(t *testing.T) {
	tests := []struct {
		name       string
		namespaced bool
		namespaces []string
		watchAll   bool
		want       []string
	}{
		{name: "cluster scoped", want: []string{""}},
		{
			name: "all namespaces", namespaced: true,
			watchAll: true, want: []string{""},
		},
		{
			name: "selected namespaces", namespaced: true,
			namespaces: []string{"a", "b"}, want: []string{"a", "b"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Namespaces(
				test.namespaced, test.namespaces, test.watchAll,
			)
			if len(got) != len(test.want) {
				t.Fatalf("Namespaces() = %v, want %v", got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("Namespaces() = %v, want %v", got, test.want)
				}
			}
		})
	}
}

func TestResourceAvailable(t *testing.T) {
	gvr := schema.GroupVersionResource{
		Group: "example.io", Version: "v1", Resource: "widgets",
	}
	client := &discoveryfake.FakeDiscovery{
		Fake: &discoverytesting.Fake{},
	}
	client.Resources = []*metav1.APIResourceList{{
		GroupVersion: "example.io/v1",
		APIResources: []metav1.APIResource{{Name: "widgets"}},
	}}
	if !ResourceAvailable(client, gvr) {
		t.Fatal("ResourceAvailable() = false, want true")
	}
}
