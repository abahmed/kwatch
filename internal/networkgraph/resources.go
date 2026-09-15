package networkgraph

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
)

var watchedResources = []struct {
	gvr        schema.GroupVersionResource
	kind       string
	namespaced bool
}{
	{schema.GroupVersionResource{
		Group: "gateway.networking.k8s.io", Version: "v1",
		Resource: "gatewayclasses",
	}, "gatewayclass", false},
	{schema.GroupVersionResource{
		Group: "gateway.networking.k8s.io", Version: "v1",
		Resource: "gateways",
	}, "gateway", true},
	{schema.GroupVersionResource{
		Group: "gateway.networking.k8s.io", Version: "v1",
		Resource: "httproutes",
	}, "httproute", true},
	{schema.GroupVersionResource{
		Group: "gateway.networking.k8s.io", Version: "v1",
		Resource: "grpcroutes",
	}, "grpcroute", true},
	{schema.GroupVersionResource{
		Group: "gateway.networking.k8s.io", Version: "v1alpha2",
		Resource: "tcproutes",
	}, "tcproute", true},
	{schema.GroupVersionResource{
		Group: "gateway.networking.k8s.io", Version: "v1alpha2",
		Resource: "tlsroutes",
	}, "tlsroute", true},
	{schema.GroupVersionResource{
		Group: "gateway.networking.k8s.io", Version: "v1",
		Resource: "referencegrants",
	}, "referencegrant", true},
}

func (m *Monitor) resourceSpecs() []dynamicwatch.ResourceSpec {
	specs := make([]dynamicwatch.ResourceSpec, 0, len(watchedResources))
	for _, watched := range watchedResources {
		kind := watched.kind
		specs = append(specs, dynamicwatch.ResourceSpec{
			GVR: watched.gvr, Namespaced: watched.namespaced,
			Handlers: cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) { m.rebuild(kind, obj) },
				UpdateFunc: func(_, obj interface{}) {
					m.rebuild(kind, obj)
				},
				DeleteFunc: func(obj interface{}) { m.remove(kind, obj) },
			},
		})
	}
	return specs
}

func (m *Monitor) watchNamespaces(namespaced bool) []string {
	if !namespaced || m.watchAll {
		return []string{""}
	}
	return dynamicwatch.Namespaces(namespaced, m.namespaces, m.watchAll)
}
