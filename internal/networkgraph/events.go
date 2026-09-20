package networkgraph

import "k8s.io/client-go/tools/cache"

func (m *Monitor) remove(kind string, obj interface{}) {
	if m.graph == nil {
		return
	}
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return
	}
	if kind == "gatewayclass" {
		ns = ""
	}
	m.graph.RemoveNode(kind, ns, name)
}
