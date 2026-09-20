package statuswatch

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/model"
)

func (m *Monitor) processStatic(obj interface{}, watched staticWatch) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok || (m.namespaceAllowed != nil &&
		!m.namespaceAllowed(u.GetNamespace())) {
		return
	}
	var sig *model.Observation
	evaluated := true
	switch watched.resource {
	case "endpoints":
		sig, evaluated = m.legacyEndpointSignal(u)
	case "certificatesigningrequest", "podcertificaterequest":
		sig = certificateSignal(u, watched.resource, m.nowTime())
	default:
		sig = failureSignal(u, watched.resource, watched.rules)
	}
	if !evaluated {
		return
	}
	if sig != nil {
		m.incidentSink.Process(sig)
	} else {
		m.resolveObject(u, watched.resource)
	}
}

func (m *Monitor) resolveStatic(obj interface{}, watched staticWatch) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		namespace, name = "", key
	}
	if m.namespaceAllowed != nil && !m.namespaceAllowed(namespace) {
		return
	}
	m.incidentSink.Resolve(
		model.NewObjectRef(watched.resource, namespace, name),
		reasonFor(watched.resource),
	)
}

// resolveObject records that a watched object no longer reports a failing
// condition.
func (m *Monitor) resolveObject(
	u *unstructured.Unstructured,
	resource string,
) {
	m.incidentSink.Resolve(
		model.NewObjectRef(resource, u.GetNamespace(), u.GetName()),
		reasonFor(resource),
	)
}

// resolve records that one reason no longer holds for a watched object.
func (m *Monitor) resolve(resource, namespace, name, reason string) {
	m.incidentSink.Resolve(
		model.NewObjectRef(resource, namespace, name), reason,
	)
}
