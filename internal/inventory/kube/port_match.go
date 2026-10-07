package kube

import (
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Attributes that let a detector compare what a Service or an Ingress
// sends traffic to with what the pods declare.
const (
	// AttrServicePortSpecs lists a Service's ports as "port:name:target"
	// entries joined by commas. The name may be empty; the target is a
	// number, or a port name when the Service targets a named port.
	AttrServicePortSpecs = "service.port.specs"
	// AttrIngressBackendPorts lists the Service ports an Ingress routes
	// to as "path<TAB>service<TAB>port" lines. The path is empty for
	// the default backend; the port is a number or a name.
	AttrIngressBackendPorts = "ingress.backend.ports"
	// AttrPodPorts lists the ports a pod's containers declare, as
	// "name=number" (or just "number" when unnamed) joined by commas.
	AttrPodPorts = "pod.ports"
)

// maxPortEntries bounds the entries kept per object.
const maxPortEntries = 64

// servicePortSpecs describes each Service port and where it leads.
func servicePortSpecs(svc *corev1.Service) string {
	var parts []string
	for _, port := range svc.Spec.Ports {
		target := strconv.Itoa(int(port.Port))
		switch {
		case port.TargetPort.StrVal != "":
			target = port.TargetPort.StrVal
		case port.TargetPort.IntVal != 0:
			target = strconv.Itoa(int(port.TargetPort.IntVal))
		}
		parts = append(parts, strconv.Itoa(int(port.Port))+":"+
			port.Name+":"+target)
		if len(parts) == maxPortEntries {
			break
		}
	}
	return strings.Join(parts, ",")
}

// ingressBackendPorts lists every Service port the Ingress routes to.
func ingressBackendPorts(ing *networkingv1.Ingress) string {
	seen := map[string]bool{}
	add := func(path string, backend *networkingv1.IngressBackend) {
		if backend == nil || backend.Service == nil ||
			backend.Service.Port.Name == actionBackendPort {
			return
		}
		port := backend.Service.Port
		value := port.Name
		if value == "" {
			value = strconv.Itoa(int(port.Number))
		}
		seen[path+"\t"+backend.Service.Name+"\t"+value] = true
	}
	add("", ing.Spec.DefaultBackend)
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			add(strings.NewReplacer("\t", " ", "\n", " ").
				Replace(path.Path), &path.Backend)
		}
	}
	lines := make([]string, 0, len(seen))
	for line := range seen {
		lines = append(lines, line)
	}
	sort.Strings(lines)
	if len(lines) > maxPortEntries {
		lines = lines[:maxPortEntries]
	}
	return strings.Join(lines, "\n")
}

// podPorts lists the ports the pod's long-running containers declare.
func podPorts(pod *corev1.Pod) string {
	var parts []string
	add := func(c corev1.Container) {
		for _, port := range c.Ports {
			entry := strconv.Itoa(int(port.ContainerPort))
			if port.Name != "" {
				entry = port.Name + "=" + entry
			}
			parts = append(parts, entry)
		}
	}
	for _, c := range pod.Spec.InitContainers {
		if c.RestartPolicy != nil &&
			*c.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			add(c)
		}
	}
	for _, c := range pod.Spec.Containers {
		add(c)
	}
	if len(parts) > maxPortEntries {
		parts = parts[:maxPortEntries]
	}
	return strings.Join(parts, ",")
}

// setPortMatchAttributes records the port facts of a Service, an
// Ingress or a pod, when it has any.
func setPortMatchAttributes(attrs map[string]inventory.Value, obj any) {
	set := func(name, value string) {
		if value != "" {
			attrs[name] = inventory.Text(value)
		}
	}
	switch o := obj.(type) {
	case *corev1.Service:
		set(AttrServicePortSpecs, servicePortSpecs(o))
	case *networkingv1.Ingress:
		set(AttrIngressBackendPorts, ingressBackendPorts(o))
	case *corev1.Pod:
		set(AttrPodPorts, podPorts(o))
	}
}

// SelectedPods returns the pods in the entity's namespace that its label
// selector matches (see selectedPods).
func SelectedPods(
	r inventory.Reader, e inventory.Entity,
) []inventory.EntityID {
	return selectedPods(r, e)
}
