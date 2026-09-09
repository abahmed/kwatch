package statuswatch

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

// serviceSpec is the part of a Service that decides whether its Endpoints
// object having no addresses is a problem.
type serviceSpec struct {
	selector  map[string]string
	clusterIP string
	typeName  string
}

// backingService reads the Service an Endpoints object belongs to, from the
// informer cache when one is wired and from the API otherwise. ok is false
// when neither can answer, which leaves the Endpoints object unjudged rather
// than reported on a guess.
func (m *Monitor) backingService(
	namespace, name string,
) (serviceSpec, bool) {
	m.mu.Lock()
	lister := m.serviceLister
	m.mu.Unlock()
	if lister != nil {
		svc, err := lister.Services(namespace).Get(name)
		if err != nil {
			return serviceSpec{}, false
		}
		return serviceSpec{
			selector:  svc.Spec.Selector,
			clusterIP: svc.Spec.ClusterIP,
			typeName:  string(svc.Spec.Type),
		}, true
	}
	services := schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "services",
	}
	service, err := m.client.Resource(services).
		Namespace(namespace).
		Get(m.ctx, name, metav1.GetOptions{})
	if err != nil {
		return serviceSpec{}, false
	}
	selector, _, _ := unstructured.NestedStringMap(
		service.Object, "spec", "selector",
	)
	clusterIP, _, _ := unstructured.NestedString(
		service.Object, "spec", "clusterIP",
	)
	typeName, _, _ := unstructured.NestedString(
		service.Object, "spec", "type",
	)
	return serviceSpec{
		selector: selector, clusterIP: clusterIP, typeName: typeName,
	}, true
}

// The two watched kinds whose failure is not written in their conditions: a
// legacy Endpoints object, which has to be read against the Service it backs,
// and a certificate request, which fails by never being issued.

func (m *Monitor) legacyEndpointSignal(
	u *unstructured.Unstructured,
) (*model.Observation, bool) {
	if u.GetNamespace() == "" {
		return nil, true
	}
	spec, ok := m.backingService(u.GetNamespace(), u.GetName())
	if !ok {
		return nil, false
	}
	if len(spec.selector) == 0 || spec.clusterIP == "" ||
		spec.clusterIP == "None" || spec.typeName == "ExternalName" {
		return nil, true
	}
	subsets, _, _ := unstructured.NestedSlice(u.Object, "subsets")
	for _, raw := range subsets {
		subset, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		addresses, _, _ := unstructured.NestedSlice(subset, "addresses")
		if len(addresses) > 0 {
			return nil, true
		}
	}
	key := u.GetNamespace() + "/" + u.GetName()
	return observe.ObjectNamed(
		"service", u.GetNamespace(), u.GetName(),
		constant.ReasonServiceNoEndpoints,
	).WithLabels(u.GetLabels()).WithHint(fmt.Sprintf(
		"legacy Endpoints object %s has no ready addresses", key,
	)), true
}

func certificateSignal(
	u *unstructured.Unstructured, resource string, now time.Time,
) *model.Observation {
	rules := map[string]map[string]bool{
		"Denied": {"True": true}, "Failed": {"True": true},
	}
	if sig := failureSignal(u, resource, rules); sig != nil {
		return sig
	}
	conditions, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	approved := false
	for _, raw := range conditions {
		condition, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := condition["type"].(string)
		status, _ := condition["status"].(string)
		if typ == "Approved" && status == "True" {
			approved = true
		}
	}
	creation := u.GetCreationTimestamp()
	if !approved || creation.IsZero() || now.Sub(creation.Time) < 10*time.Minute {
		return nil
	}
	certificateField := "certificate"
	if resource == "podcertificaterequest" {
		certificateField = "certificateChain"
	}
	certificate, _, _ := unstructured.NestedString(
		u.Object, "status", certificateField,
	)
	if certificate == "" {
		return observeObject(u, resource).WithHint(
			"certificate request was approved but the signer has not" +
				" issued a certificate after 10 minutes",
		)
	}
	block, _ := pem.Decode([]byte(certificate))
	if block == nil {
		return nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil || cert.NotAfter.Sub(now) > 7*24*time.Hour {
		return nil
	}
	return observeObject(u, resource).WithHint(fmt.Sprintf(
		"issued certificate expires at %s",
		cert.NotAfter.UTC().Format(time.RFC3339),
	))
}
