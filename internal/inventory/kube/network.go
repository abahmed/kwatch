package kube

import (
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Attribute names specific to networking entities.
const (
	AttrServiceType     = "service.type"
	AttrHeadless        = "service.headless"
	AttrSelector        = "selector"
	AttrPorts           = "ports"
	AttrLoadBalancer    = "loadbalancer.assigned"
	AttrEndpoints       = "endpoints"
	AttrEndpointsReady  = "endpoints.ready"
	AttrIngressClass    = "ingress.class"
	AttrExternalName    = "external.name"
	AttrTargetPorts     = "target.ports"
	AttrEndpointPorts   = "endpoint.ports"
	endpointServiceName = discoveryv1.LabelServiceName
)

// skipProbeAnnotation opts a Service out of automatic probing.
const skipProbeAnnotation = "kwatch.io/skip-probe"

// ServiceSchema describes Services. Which pods back a Service comes from
// its EndpointSlices, not from re-evaluating the selector.
type ServiceSchema struct{}

// Kind implements Schema.
func (ServiceSchema) Kind() inventory.Kind { return KindService }

// RelationTypes implements Schema.
func (ServiceSchema) RelationTypes() []inventory.RelationType { return nil }

// Describe implements Schema.
func (ServiceSchema) Describe(obj any) (Description, bool) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return Description{}, false
	}
	attrs := map[string]inventory.Value{
		AttrServiceType:  inventory.Text(string(svc.Spec.Type)),
		AttrSelector:     inventory.Text(labelText(svc.Spec.Selector)),
		AttrPorts:        inventory.Text(servicePorts(svc)),
		AttrLoadBalancer: inventory.Bool(loadBalancerAssigned(svc)),
		AttrTargetPorts:  inventory.Text(targetPorts(svc)),
	}
	if svc.Annotations[skipProbeAnnotation] == "true" {
		attrs[AttrSkipProbe] = inventory.Bool(true)
	}
	if svc.Spec.ClusterIP == corev1.ClusterIPNone {
		attrs[AttrHeadless] = inventory.Bool(true)
	}
	if svc.Spec.ExternalName != "" {
		attrs[AttrExternalName] = inventory.Text(svc.Spec.ExternalName)
	}
	return Description{
		ID: objectID(KindService, svc), UID: string(svc.UID),
		Attributes: attrs,
	}, true
}

// Diff implements Schema.
func (ServiceSchema) Diff(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*corev1.Service)
	after, ok2 := new.(*corev1.Service)
	if !ok1 || !ok2 {
		return nil
	}
	var fields []inventory.FieldChange
	add := func(path, b, a string) {
		if b != a {
			fields = append(fields, inventory.FieldChange{
				Path: path, Before: b, After: a,
			})
		}
	}
	add("spec.selector", labelText(before.Spec.Selector),
		labelText(after.Spec.Selector))
	add("spec.ports", servicePorts(before), servicePorts(after))
	add("spec.type", string(before.Spec.Type), string(after.Spec.Type))
	return fields
}

// EndpointSliceSchema describes EndpointSlices: they back a Service and
// route to the pods they list.
type EndpointSliceSchema struct{}

// Kind implements Schema.
func (EndpointSliceSchema) Kind() inventory.Kind { return KindEndpointSlice }

// RelationTypes implements Schema.
func (EndpointSliceSchema) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{inventory.Backs, inventory.RoutesTo}
}

// Describe implements Schema.
func (EndpointSliceSchema) Describe(obj any) (Description, bool) {
	slice, ok := obj.(*discoveryv1.EndpointSlice)
	if !ok {
		return Description{}, false
	}
	ready := 0
	rel := relations{}
	rel.add(inventory.Backs, inventory.CoreID(
		KindService, slice.Namespace, slice.Labels[endpointServiceName]))
	for _, endpoint := range slice.Endpoints {
		if endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready {
			ready++
		}
		if ref := endpoint.TargetRef; ref != nil && ref.Kind == "Pod" {
			rel.add(inventory.RoutesTo,
				inventory.CoreID(KindPod, ref.Namespace, ref.Name))
		}
	}
	return Description{
		ID: objectID(KindEndpointSlice, slice), UID: string(slice.UID),
		Attributes: map[string]inventory.Value{
			AttrEndpoints:      inventory.Number(float64(len(slice.Endpoints))),
			AttrEndpointsReady: inventory.Number(float64(ready)),
			AttrEndpointPorts:  inventory.Text(endpointPorts(slice)),
		},
		Relations: rel,
	}, true
}

// Diff implements Schema. Endpoint churn is state, never a change.
func (EndpointSliceSchema) Diff(_, _ any) []inventory.FieldChange {
	return nil
}

// IngressSchema describes Ingresses.
type IngressSchema struct{}

// Kind implements Schema.
func (IngressSchema) Kind() inventory.Kind { return KindIngress }

// RelationTypes implements Schema.
func (IngressSchema) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{inventory.RoutesTo, inventory.References}
}

// Describe implements Schema.
func (IngressSchema) Describe(obj any) (Description, bool) {
	ing, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return Description{}, false
	}
	rel := relations{}
	for _, svc := range ingressServices(ing) {
		rel.add(inventory.RoutesTo,
			inventory.CoreID(KindService, ing.Namespace, svc))
	}
	for _, tls := range ing.Spec.TLS {
		rel.add(inventory.References,
			inventory.CoreID(KindSecret, ing.Namespace, tls.SecretName))
	}
	attrs := map[string]inventory.Value{
		AttrLoadBalancer: inventory.Bool(
			len(ing.Status.LoadBalancer.Ingress) > 0),
	}
	if ing.Spec.IngressClassName != nil {
		attrs[AttrIngressClass] = inventory.Text(*ing.Spec.IngressClassName)
	}
	return Description{
		ID: objectID(KindIngress, ing), UID: string(ing.UID),
		Attributes: attrs, Relations: rel,
	}, true
}

// Diff implements Schema.
func (IngressSchema) Diff(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*networkingv1.Ingress)
	after, ok2 := new.(*networkingv1.Ingress)
	if !ok1 || !ok2 {
		return nil
	}
	b := strings.Join(ingressServices(before), ",")
	a := strings.Join(ingressServices(after), ",")
	if b == a {
		return nil
	}
	return []inventory.FieldChange{{
		Path: "spec.backends", Before: b, After: a,
	}}
}

const actionBackendPort = "use-annotation"

func ingressServices(ing *networkingv1.Ingress) []string {
	seen := map[string]bool{}
	add := func(backend *networkingv1.IngressBackend) {
		// A use-annotation port names an AWS Load Balancer Controller
		// action (such as ssl-redirect), which is no Service.
		if backend != nil && backend.Service != nil &&
			backend.Service.Port.Name != actionBackendPort {
			seen[backend.Service.Name] = true
		}
	}
	add(ing.Spec.DefaultBackend)
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			add(&path.Backend)
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func labelText(labels map[string]string) string {
	parts := make([]string, 0, len(labels))
	for key, value := range labels {
		parts = append(parts, key+"="+value)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func servicePorts(svc *corev1.Service) string {
	parts := make([]string, 0, len(svc.Spec.Ports))
	for _, port := range svc.Spec.Ports {
		parts = append(parts, strconv.Itoa(int(port.Port))+"/"+
			string(port.Protocol)+"->"+port.TargetPort.String())
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func loadBalancerAssigned(svc *corev1.Service) bool {
	for _, ingress := range svc.Status.LoadBalancer.Ingress {
		if ingress.IP != "" || ingress.Hostname != "" {
			return true
		}
	}
	return false
}

func selectorText(selector *metav1.LabelSelector) string {
	if selector == nil {
		return ""
	}
	return metav1.FormatLabelSelector(selector)
}

func resourceListText(list corev1.ResourceList) string {
	parts := make([]string, 0, len(list))
	for name, quantity := range list {
		parts = append(parts, string(name)+"="+quantity.String())
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// targetPorts lists each Service port's target as "name" or "number",
// the form EndpointSlices publish.
func targetPorts(svc *corev1.Service) string {
	parts := make([]string, 0, len(svc.Spec.Ports))
	for _, port := range svc.Spec.Ports {
		switch {
		case port.TargetPort.StrVal != "":
			parts = append(parts, port.TargetPort.StrVal)
		case port.TargetPort.IntVal != 0:
			parts = append(parts, strconv.Itoa(int(port.TargetPort.IntVal)))
		default:
			parts = append(parts, strconv.Itoa(int(port.Port)))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// endpointPorts lists an EndpointSlice's port names and numbers.
func endpointPorts(slice *discoveryv1.EndpointSlice) string {
	var parts []string
	for _, port := range slice.Ports {
		if port.Name != nil && *port.Name != "" {
			parts = append(parts, *port.Name)
		}
		if port.Port != nil {
			parts = append(parts, strconv.Itoa(int(*port.Port)))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
