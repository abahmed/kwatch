package statuswatch

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/constant"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

func (m *Monitor) processAPIService(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	if sig := failureSignal(u, "apiservice", m.conditionRules); sig != nil {
		m.incidentSink.Process(sig)
	} else {
		m.resolve(
			"apiservice", "", u.GetName(), constant.ReasonAPIServiceFailure,
		)
	}
}

func (m *Monitor) resolveAPIService(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err == nil && name != "" {
		m.resolve("apiservice", "", name, constant.ReasonAPIServiceFailure)
	}
}

func (m *Monitor) processCR(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}
	if m.namespaceAllowed != nil && !m.namespaceAllowed(u.GetNamespace()) {
		return
	}
	m.rebuildGraph(u)
	if sig := failureSignal(u, "customresource", m.conditionRules); sig != nil {
		m.incidentSink.Process(sig)
	} else {
		m.resolve(
			"customresource", u.GetNamespace(), u.GetName(),
			constant.ReasonCustomResourceFailure,
		)
	}
}

func (m *Monitor) resolveCR(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil ||
		(m.namespaceAllowed != nil && !m.namespaceAllowed(namespace)) {
		return
	}
	if m.graph != nil {
		m.graph.RemoveNode("customresource", namespace, name)
	}
	m.resolve(
		"customresource", namespace, name,
		constant.ReasonCustomResourceFailure,
	)
}

func (m *Monitor) rebuildGraph(u *unstructured.Unstructured) {
	if m.graph == nil {
		return
	}
	targets := make([]kwcontext.EdgeTarget, 0, len(u.GetOwnerReferences()))
	for _, ref := range u.GetOwnerReferences() {
		if ref.Name == "" || ref.Kind == "" {
			continue
		}
		targets = append(targets, kwcontext.EdgeTarget{
			Kind:      strings.ToLower(ref.Kind),
			Namespace: u.GetNamespace(), Name: ref.Name,
			Type: "owned_by",
		})
	}
	for _, rule := range m.graphReferences {
		for _, name := range nestedStringValues(u.Object, rule.path) {
			targets = append(targets, kwcontext.EdgeTarget{
				Kind: rule.kind, Namespace: u.GetNamespace(), Name: name,
				Type: "references",
			})
		}
	}
	m.graph.ReplaceOutgoingEdges(
		"customresource", u.GetNamespace(), u.GetName(), targets,
	)
}
