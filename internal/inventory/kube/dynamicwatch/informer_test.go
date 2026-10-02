package dynamicwatch

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	metadatafake "k8s.io/client-go/metadata/fake"
)

var widgets = schema.GroupVersionResource{
	Group: "example.com", Version: "v1", Resource: "widgets",
}

func keep(obj any) (any, error) { return obj, nil }

func TestNewInformerNeedsAClient(t *testing.T) {
	if _, _, err := NewInformer(nil, 0, "", widgets, nil); err == nil {
		t.Fatal("a missing dynamic client must be an error")
	}
	if _, err := NewMetadataInformer(nil, 0, widgets, nil); err == nil {
		t.Fatal("a missing metadata client must be an error")
	}
}

func TestNewInformerBuildsTransformedInformers(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme, map[schema.GroupVersionResource]string{
			widgets: "WidgetList",
		})
	factory, informer, err := NewInformer(client, 0, "ns", widgets, keep)
	if err != nil || factory == nil || informer == nil {
		t.Fatalf("NewInformer = %v, %v, %v", factory, informer, err)
	}
	metadata, err := NewMetadataInformer(
		metadatafake.NewSimpleMetadataClient(scheme), 0, widgets, keep)
	if err != nil || metadata == nil {
		t.Fatalf("NewMetadataInformer = %v, %v", metadata, err)
	}
}
