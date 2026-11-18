package kube

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Kubernetes publishes some objects into every namespace by itself, a
// moment after the namespace is created: the kube-root-ca.crt ConfigMap
// (the root CA of the service-account token volume every pod mounts)
// and the default ServiceAccount. A pod created with its namespace can
// reference them before they exist, and their watch events can reach
// kwatch later than the pod's. Missing during the namespace's first
// moments is that ordering effect, never a cause; missing well beyond
// it is a real finding.

// BootstrapPublish is how long after a namespace is created its
// bootstrap objects may still be missing without being a finding. The
// controllers publish them within seconds; the margin covers a busy API
// server and a lagging watch.
const BootstrapPublish = 2 * time.Minute

// Names of the objects the control plane publishes into every namespace.
const (
	RootCAConfigMap       = "kube-root-ca.crt"
	DefaultServiceAccount = "default"
)

// PublishedByControlPlane reports an object Kubernetes creates by itself
// in every namespace.
func PublishedByControlPlane(id inventory.EntityID) bool {
	switch id.Kind {
	case KindConfigMap:
		return id.Name == RootCAConfigMap
	case KindAccount:
		return id.Name == DefaultServiceAccount
	}
	return false
}

// BootstrapPending is how long a missing object the control plane
// publishes (PublishedByControlPlane) may still be missing as a mere
// ordering effect: the rest of BootstrapPublish since its namespace was
// created. It is zero for any other object, and when the namespace or
// its creation time is unknown.
func BootstrapPending(
	model inventory.Reader, id inventory.EntityID, now time.Time,
) time.Duration {
	if model == nil || id.Namespace == "" || !PublishedByControlPlane(id) {
		return 0
	}
	created, ok := model.Attribute(
		inventory.CoreID(KindNamespace, "", id.Namespace), AttrCreated)
	if !ok {
		return 0
	}
	return max(created.Value.AsTime().Add(BootstrapPublish).Sub(now), 0)
}
