package handler

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
)

var defaultServiceSustainedSeconds float64 = 60

func DetectServiceEndpointIssue(
	svc *corev1.Service,
	epSlices []*discoveryv1.EndpointSlice,
) *event.Signal {
	if svc == nil {
		return nil
	}
	if len(svc.Spec.Selector) == 0 {
		return nil
	}
	if svc.Spec.ClusterIP == "None" || svc.Spec.ClusterIP == "" {
		return nil
	}
	if svc.Spec.Type == corev1.ServiceTypeExternalName {
		return nil
	}

	hasReady := false
	for _, slice := range epSlices {
		for _, ep := range slice.Endpoints {
			if endpointCanReceiveTraffic(ep) {
				hasReady = true
				break
			}
		}
		if hasReady {
			break
		}
	}

	if !hasReady {
		key := svc.Namespace + "/" + svc.Name
		return &event.Signal{
			Resource:  "service",
			Namespace: svc.Namespace,
			Reason:    constant.ReasonServiceNoEndpoints,
			Owner:     key,
			PodName:   svc.Name,
			Labels:    svc.Labels,
			Hint: fmt.Sprintf(
				"service %s has selectors but no ready endpoints",
				key,
			),
		}
	}
	return nil
}

func DetectServicePortIssue(
	svc *corev1.Service,
	epSlices []*discoveryv1.EndpointSlice,
) *event.Signal {
	if svc == nil || len(svc.Spec.Ports) == 0 || len(epSlices) == 0 {
		return nil
	}
	names, numbers := servicePortExpectations(svc)
	observedNames, observedNumbers := endpointPortObservations(epSlices)
	for name := range names {
		if !observedNames[name] {
			return servicePortMismatch(svc, fmt.Sprintf("named port %q is not published by its EndpointSlices", name))
		}
	}
	for key := range numbers {
		if !observedNumbers[key] {
			return servicePortMismatch(svc, fmt.Sprintf("target port %s is not published by its EndpointSlices", key))
		}
	}
	return nil
}

func DetectServiceStatusIssue(svc *corev1.Service, now time.Time) *event.Signal {
	if svc == nil {
		return nil
	}
	key := svc.Namespace + "/" + svc.Name
	for _, condition := range svc.Status.Conditions {
		if condition.Status != metav1.ConditionFalse {
			continue
		}
		hint := string(condition.Type) + ": " + condition.Reason
		if condition.Message != "" {
			hint += " — " + condition.Message
		}
		return &event.Signal{Resource: "service", Namespace: svc.Namespace, PodName: svc.Name,
			Owner: key, Reason: constant.ReasonLoadBalancerPending, Labels: svc.Labels, Hint: hint}
	}
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer || len(svc.Status.LoadBalancer.Ingress) > 0 {
		return nil
	}
	if now.Sub(svc.CreationTimestamp.Time) < time.Duration(defaultServiceSustainedSeconds)*time.Second {
		return nil
	}
	return &event.Signal{Resource: "service", Namespace: svc.Namespace, PodName: svc.Name,
		Owner: key, Reason: constant.ReasonLoadBalancerPending, Labels: svc.Labels,
		Hint: fmt.Sprintf("LoadBalancer service %s has no provisioned ingress address", key)}
}

func servicePortExpectations(svc *corev1.Service) (map[string]bool, map[string]bool) {
	names := make(map[string]bool)
	numbers := make(map[string]bool)
	for _, port := range svc.Spec.Ports {
		if port.Name != "" {
			names[port.Name] = true
		}
		if port.TargetPort.StrVal == "" {
			target := port.TargetPort.IntVal
			if target == 0 {
				target = port.Port
			}
			numbers[servicePortKey(port.Protocol, target)] = true
		}
	}
	return names, numbers
}

func endpointPortObservations(epSlices []*discoveryv1.EndpointSlice) (map[string]bool, map[string]bool) {
	names := make(map[string]bool)
	numbers := make(map[string]bool)
	for _, slice := range epSlices {
		for _, port := range slice.Ports {
			if port.Name != nil {
				names[*port.Name] = true
			}
			if port.Port == nil {
				continue
			}
			protocol := corev1.ProtocolTCP
			if port.Protocol != nil {
				protocol = *port.Protocol
			}
			numbers[servicePortKey(protocol, *port.Port)] = true
		}
	}
	return names, numbers
}

func servicePortMismatch(svc *corev1.Service, detail string) *event.Signal {
	key := svc.Namespace + "/" + svc.Name
	return &event.Signal{Resource: "service", Namespace: svc.Namespace, PodName: svc.Name,
		Owner: key, Reason: constant.ReasonServicePortMismatch, Labels: svc.Labels,
		Hint: fmt.Sprintf("service %s %s", key, detail)}
}

func servicePortKey(protocol corev1.Protocol, port int32) string {
	if protocol == "" {
		protocol = corev1.ProtocolTCP
	}
	return fmt.Sprintf("%s/%d", protocol, port)
}

// endpointCanReceiveTraffic follows EndpointSlice conditions. A terminating
// endpoint is not a healthy destination for new Service traffic, while a
// missing Ready condition is usable unless Serving explicitly says false.
func endpointCanReceiveTraffic(ep discoveryv1.Endpoint) bool {
	if ep.Conditions.Terminating != nil && *ep.Conditions.Terminating {
		return false
	}
	if ep.Conditions.Ready != nil {
		return *ep.Conditions.Ready
	}
	return ep.Conditions.Serving == nil || *ep.Conditions.Serving
}

func (h *handler) ProcessService(key string, deleted bool) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid service key %q: %w", key, err)
	}
	if deleted {
		h.correlator.ResolveByResource("service", namespace+"/"+name)
		return nil
	}
	svc, err := h.listers.Service.Services(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			h.correlator.ResolveByResource("service", namespace+"/"+name)
			return nil
		}
		return fmt.Errorf(
			"failed to get service %s/%s from cache: %w",
			namespace,
			name,
			err,
		)
	}
	return h.ProcessServiceObject(svc, false)
}

func (h *handler) ProcessServiceObject(
	svc *corev1.Service,
	deleted bool,
) error {
	if svc == nil {
		return nil
	}

	if deleted {
		h.clearServiceNoEndpoints(svc.Namespace, svc.Name)
		h.correlator.ResolveByResource("service", svc.Namespace+"/"+svc.Name)
		return nil
	}

	sel := labels.Set{"kubernetes.io/service-name": svc.Name}.AsSelector()
	epSlices, err := h.listers.EndpointSlice.EndpointSlices(
		svc.Namespace,
	).List(
		sel,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to list endpoint slices for %s/%s: %w",
			svc.Namespace,
			svc.Name,
			err,
		)
	}

	sig := DetectServiceEndpointIssue(svc, epSlices)
	portSig := DetectServicePortIssue(svc, epSlices)
	statusSig := DetectServiceStatusIssue(svc, h.now())
	key := svc.Namespace + "/" + svc.Name
	if sig != nil {
		first := h.markServiceNoEndpoints(key)
		sustained := time.Duration(defaultServiceSustainedSeconds) * time.Second
		if h.now().Sub(first) >= sustained {
			h.signalEvent(sig)
		}
	} else {
		h.clearServiceNoEndpoints(svc.Namespace, svc.Name)
		h.correlator.MarkResolved(
			correlation.BuildKey(
				svc.Namespace,
				svc.Namespace+"/"+svc.Name,
				constant.ReasonServiceNoEndpoints,
				"",
			),
		)
	}
	if portSig != nil {
		h.signalEvent(portSig)
	} else {
		h.correlator.MarkResolved(correlation.BuildKey(
			svc.Namespace, key, constant.ReasonServicePortMismatch, "",
		))
	}
	if statusSig != nil {
		h.signalEvent(statusSig)
	} else {
		h.correlator.MarkResolved(correlation.BuildKey(
			svc.Namespace, key, constant.ReasonLoadBalancerPending, "",
		))
	}
	return nil
}

func (h *handler) markServiceNoEndpoints(key string) time.Time {
	return h.fs.serviceNoEndpoint.mark(key, h.now())
}

func (h *handler) clearServiceNoEndpoints(namespace, name string) {
	h.fs.serviceNoEndpoint.clear(correlation.OwnerPath(namespace, name))
}
