package kube

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Entity kinds produced by this package.
const (
	KindPod           inventory.Kind = "pod"
	KindContainer     inventory.Kind = "container"
	KindNode          inventory.Kind = "node"
	KindDeployment    inventory.Kind = "deployment"
	KindReplicaSet    inventory.Kind = "replicaset"
	KindStatefulSet   inventory.Kind = "statefulset"
	KindDaemonSet     inventory.Kind = "daemonset"
	KindJob           inventory.Kind = "job"
	KindCronJob       inventory.Kind = "cronjob"
	KindHPA           inventory.Kind = "horizontalpodautoscaler"
	KindService       inventory.Kind = "service"
	KindEndpointSlice inventory.Kind = "endpointslice"
	KindIngress       inventory.Kind = "ingress"
	KindSecret        inventory.Kind = "secret"
	KindConfigMap     inventory.Kind = "configmap"
	KindAccount       inventory.Kind = "serviceaccount"
	KindPVC           inventory.Kind = "persistentvolumeclaim"
	KindPV            inventory.Kind = "persistentvolume"
	KindStorageClass  inventory.Kind = "storageclass"
	KindPriorityClass inventory.Kind = "priorityclass"
	KindRuntimeClass  inventory.Kind = "runtimeclass"
	KindNamespace     inventory.Kind = "namespace"
	KindImage         inventory.Kind = "image"
	KindRegistry      inventory.Kind = "registry"
	KindZone          inventory.Kind = "zone"
	KindNodePool      inventory.Kind = "nodepool"
	// KindLease is a coordination Lease read by the prober, outside the
	// node leases and the control-plane leaders it already covers: the
	// leader election of an operator or controller.
	KindLease inventory.Kind = "lease"
	// KindExternalEndpoint is a network endpoint outside the objects
	// kwatch watches, named "host:port": a database, a queue, a
	// third-party API. Pods call it (their configuration or their error
	// text names it), and kwatch's own probe reports whether it accepts
	// connections.
	KindExternalEndpoint inventory.Kind = "external-endpoint"
)

// builtinGroups are the API groups kube-apiserver serves itself. They
// all map to the built-in entity group: their kinds are unique across
// them, except kinds served twice for the same objects (Event in the
// core and events.k8s.io groups, Ingress in extensions and
// networking.k8s.io), which must share one entity anyway.
var builtinGroups = map[string]bool{
	"": true, "apps": true, "batch": true, "autoscaling": true,
	"policy": true, "extensions": true,
	"admissionregistration.k8s.io": true,
	"apiextensions.k8s.io":         true,
	"apiregistration.k8s.io":       true,
	"authentication.k8s.io":        true,
	"authorization.k8s.io":         true,
	"certificates.k8s.io":          true,
	"coordination.k8s.io":          true,
	"discovery.k8s.io":             true,
	"events.k8s.io":                true,
	"flowcontrol.apiserver.k8s.io": true,
	"internal.apiserver.k8s.io":    true,
	"networking.k8s.io":            true,
	"node.k8s.io":                  true,
	"rbac.authorization.k8s.io":    true,
	"resource.k8s.io":              true,
	"scheduling.k8s.io":            true,
	"storage.k8s.io":               true,
	"storagemigration.k8s.io":      true,
}

// KindFor maps a Kubernetes Kind ("ReplicaSet") to an entity kind.
func KindFor(kubernetesKind string) inventory.Kind {
	return inventory.Kind(strings.ToLower(kubernetesKind))
}

// GroupFor maps an API group to an entity group: empty for built-in
// groups, the group itself otherwise.
func GroupFor(apiGroup string) string {
	if builtinGroups[apiGroup] {
		return ""
	}
	return apiGroup
}

// GroupForAPIVersion maps an apiVersion ("apps/v1", "v1",
// "gateway.networking.k8s.io/v1") to an entity group. An empty or
// malformed apiVersion is taken as built-in.
func GroupForAPIVersion(apiVersion string) string {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return ""
	}
	return GroupFor(gv.Group)
}

// EntityFor identifies an object from its apiVersion, Kubernetes Kind,
// namespace and name, as found in owner and object references.
func EntityFor(
	apiVersion, kubernetesKind, namespace, name string,
) inventory.EntityID {
	return inventory.NewEntityID(GroupForAPIVersion(apiVersion),
		KindFor(kubernetesKind), namespace, name)
}

// AttrKindName is how the API spells the entity's Kind ("DatadogAgent"),
// set only when the spelling says more than the lower-case entity kind,
// so messages can name a custom resource the way its CRD does.
const AttrKindName = "kind.name"

// KindNameAttributes returns the attributes (maybe none) that record the
// spelling of a Kubernetes Kind. Only a name with a capital inside it
// ("DatadogAgent") says more than the entity kind; "Gateway" reads the
// same in lower case.
func KindNameAttributes(
	kubernetesKind string,
) map[string]inventory.Value {
	attrs := map[string]inventory.Value{}
	if len(kubernetesKind) > 1 &&
		kubernetesKind[1:] != strings.ToLower(kubernetesKind[1:]) {
		attrs[AttrKindName] = inventory.Text(kubernetesKind)
	}
	return attrs
}

// WorkloadKinds are the controller workloads that run pods and report
// replica counts.
var WorkloadKinds = []inventory.Kind{
	KindDeployment, KindStatefulSet, KindDaemonSet,
}
