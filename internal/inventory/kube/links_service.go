package kube

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Entity kinds of objects served by a Service.
const (
	KindCRD        inventory.Kind = "customresourcedefinition"
	KindAPIService inventory.Kind = "apiservice"
)

// servedByExtractors read the Service behind objects the API server calls
// out to. Each returns the Service's namespace and name pairs.
var servedByExtractors = map[inventory.Kind]func(
	*unstructured.Unstructured) []inventory.EntityID{
	KindCRD:             conversionService,
	KindAPIService:      apiServiceBackend,
	KindMutatingWebhook: webhookServices,
	KindValidatingHook:  webhookServices,
}

// servedByLinks returns the Services serving u. Only built-in kinds are
// read: a custom kind that happens to share a name is not a webhook.
func servedByLinks(
	group string, kind inventory.Kind, u *unstructured.Unstructured,
) []inventory.EntityID {
	extract, ok := servedByExtractors[kind]
	if !ok || group != "" {
		return nil
	}
	return extract(u)
}

func conversionService(u *unstructured.Unstructured) []inventory.EntityID {
	svc, _, _ := unstructured.NestedMap(u.Object, "spec", "conversion",
		"webhook", "clientConfig", "service")
	return []inventory.EntityID{serviceRef(svc)}
}

func apiServiceBackend(u *unstructured.Unstructured) []inventory.EntityID {
	svc, _, _ := unstructured.NestedMap(u.Object, "spec", "service")
	return []inventory.EntityID{serviceRef(svc)}
}

func webhookServices(u *unstructured.Unstructured) []inventory.EntityID {
	hooks, _, _ := unstructured.NestedSlice(u.Object, "webhooks")
	out := make([]inventory.EntityID, 0, len(hooks))
	for _, hook := range hooks {
		m, _ := hook.(map[string]any)
		svc, _, _ := unstructured.NestedMap(m, "clientConfig", "service")
		out = append(out, serviceRef(svc))
	}
	return out
}

// serviceRef reads a {namespace, name} Service reference. A reference
// without a namespace names nothing: the API server requires one.
func serviceRef(svc map[string]any) inventory.EntityID {
	ns, name := str(svc, "namespace"), str(svc, "name")
	if ns == "" || name == "" {
		return inventory.EntityID{}
	}
	return inventory.CoreID(KindService, ns, name)
}

// selectorAttr reads a pod label selector from spec.selector or
// spec.podSelector and renders it in label-selector syntax. Both the
// LabelSelector form (matchLabels, matchExpressions) and the plain map
// form of a Service selector are read. An empty or invalid selector
// returns "": a custom resource's empty selector is ambiguous, so it
// never selects every pod.
func selectorAttr(u *unstructured.Unstructured) string {
	for _, field := range []string{"selector", "podSelector"} {
		raw, found, _ := unstructured.NestedFieldNoCopy(
			u.Object, "spec", field)
		if !found {
			continue
		}
		if text := selectorValue(raw); text != "" {
			return text
		}
	}
	return ""
}

func selectorValue(raw any) string {
	switch v := raw.(type) {
	case string:
		if _, err := labels.Parse(v); err != nil {
			return ""
		}
		return v
	case map[string]any:
		if _, ok := v["matchLabels"]; ok {
			return labelSelectorText(v)
		}
		if _, ok := v["matchExpressions"]; ok {
			return labelSelectorText(v)
		}
		return plainSelectorText(v)
	}
	return ""
}

func labelSelectorText(m map[string]any) string {
	var sel metav1.LabelSelector
	if runtime.DefaultUnstructuredConverter.FromUnstructured(
		m, &sel) != nil {
		return ""
	}
	selector, err := metav1.LabelSelectorAsSelector(&sel)
	if err != nil || selector.Empty() {
		return ""
	}
	return selector.String()
}

func plainSelectorText(m map[string]any) string {
	set := make(map[string]string, len(m))
	for key, value := range m {
		text, ok := value.(string)
		if !ok {
			return ""
		}
		set[key] = text
	}
	if len(set) == 0 {
		return ""
	}
	if _, err := labels.ValidatedSelectorFromSet(set); err != nil {
		return ""
	}
	return labelText(set)
}
