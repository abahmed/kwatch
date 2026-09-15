package network

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultServiceSustainedSeconds is the default delay before reporting a
// Service without a usable LoadBalancer address.
const DefaultServiceSustainedSeconds = 60

// DetectServiceEndpointIssue reports a Service with no ready endpoints.
func DetectServiceEndpointIssue(
	svc *corev1.Service,
	epSlices []*discoveryv1.EndpointSlice,
) *model.Observation {
	if svc == nil || len(svc.Spec.Selector) == 0 ||
		svc.Spec.ClusterIP == "None" || svc.Spec.ClusterIP == "" ||
		svc.Spec.Type == corev1.ServiceTypeExternalName {
		return nil
	}
	for _, slice := range epSlices {
		for _, endpoint := range slice.Endpoints {
			if EndpointCanReceiveTraffic(endpoint) {
				return nil
			}
		}
	}
	key := svc.Namespace + "/" + svc.Name
	return observe.Object(
		"service", svc, constant.ReasonServiceNoEndpoints,
	).WithHint(fmt.Sprintf(
		"service %s has selectors but no ready endpoints", key,
	))
}

// DetectServicePortIssue reports Service ports absent from EndpointSlices.
func DetectServicePortIssue(
	svc *corev1.Service,
	epSlices []*discoveryv1.EndpointSlice,
) *model.Observation {
	if svc == nil || len(svc.Spec.Ports) == 0 || len(epSlices) == 0 {
		return nil
	}
	names, numbers := servicePortExpectations(svc)
	observedNames, observedNumbers := endpointPortObservations(epSlices)
	for name := range names {
		if !observedNames[name] {
			return servicePortMismatch(
				svc,
				fmt.Sprintf(
					"named port %q is not published by its EndpointSlices",
					name,
				),
			)
		}
	}
	for port := range numbers {
		if !observedNumbers[port] {
			return servicePortMismatch(
				svc,
				fmt.Sprintf(
					"target port %s is not published by its EndpointSlices",
					port,
				),
			)
		}
	}
	return nil
}

// DetectServiceStatusIssue reports failed Service status or a pending
// LoadBalancer after the supplied sustain window.
func DetectServiceStatusIssue(
	svc *corev1.Service,
	now time.Time,
	sustainedSeconds float64,
) *model.Observation {
	if svc == nil {
		return nil
	}
	key := svc.Namespace + "/" + svc.Name
	for _, condition := range svc.Status.Conditions {
		if condition.Status != metav1.ConditionFalse &&
			condition.Status != metav1.ConditionUnknown {
			continue
		}
		hint := string(condition.Type) + ": " + condition.Reason
		if condition.Message != "" {
			hint += " — " + condition.Message
		}
		return observe.Object(
			"service", svc, constant.ReasonLoadBalancerPending,
		).WithHint(hint)
	}
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer ||
		serviceHasLoadBalancerAddress(svc) {
		return nil
	}
	if now.Sub(svc.CreationTimestamp.Time) <
		time.Duration(sustainedSeconds)*time.Second {
		return nil
	}
	return observe.Object(
		"service", svc, constant.ReasonLoadBalancerPending,
	).WithHint(fmt.Sprintf(
		"LoadBalancer service %s has no provisioned ingress address", key,
	))
}

func serviceHasLoadBalancerAddress(svc *corev1.Service) bool {
	for _, ingress := range svc.Status.LoadBalancer.Ingress {
		if ingress.IP != "" || ingress.Hostname != "" {
			return true
		}
	}
	return false
}

func servicePortExpectations(
	svc *corev1.Service,
) (map[string]bool, map[string]bool) {
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

func endpointPortObservations(
	epSlices []*discoveryv1.EndpointSlice,
) (map[string]bool, map[string]bool) {
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

func servicePortMismatch(
	svc *corev1.Service,
	detail string,
) *model.Observation {
	key := svc.Namespace + "/" + svc.Name
	return observe.Object(
		"service", svc, constant.ReasonServicePortMismatch,
	).WithHint(fmt.Sprintf("service %s %s", key, detail))
}

func servicePortKey(protocol corev1.Protocol, port int32) string {
	if protocol == "" {
		protocol = corev1.ProtocolTCP
	}
	return fmt.Sprintf("%s/%d", protocol, port)
}

// EndpointCanReceiveTraffic follows EndpointSlice readiness semantics.
func EndpointCanReceiveTraffic(endpoint discoveryv1.Endpoint) bool {
	if endpoint.Conditions.Terminating != nil &&
		*endpoint.Conditions.Terminating {
		return false
	}
	if endpoint.Conditions.Ready != nil {
		return *endpoint.Conditions.Ready
	}
	return endpoint.Conditions.Serving == nil || *endpoint.Conditions.Serving
}
