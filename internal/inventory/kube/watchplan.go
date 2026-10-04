package kube

import (
	"github.com/abahmed/kwatch/internal/inventory"

	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// WatchMode is how one resource type is watched. It is recorded on every
// entity as AttrWatchMode so detection knows which attributes to expect.
type WatchMode string

// Watch modes, from richest to cheapest.
const (
	// WatchFull is a typed informer with a kind-specific translator.
	WatchFull WatchMode = "full"
	// WatchHashed is a typed informer whose cache keeps a hash of the
	// object's data instead of the data (Secrets and ConfigMaps).
	WatchHashed WatchMode = "hashed"
	// WatchStatus is a dynamic informer whose cache keeps metadata,
	// status and a small reference-bearing spec subset.
	WatchStatus WatchMode = "status"
	// WatchMetadata is a metadata-only informer: identity, owners,
	// generation, deletion and finalizers.
	WatchMetadata WatchMode = "metadata"
)

// Dynamic watch budget. Typed kinds (the registrations in source.go) are
// always watched and never count against it.
//
// Resource types discovered beyond the typed kinds are ordered by tier,
// then by group and resource name, and the first DefaultResourceBudget
// are watched; the rest are skipped and counted (DynamicStatus.Skipped):
//
//  0. discovery anchors: CustomResourceDefinitions and APIServices,
//     whose changes trigger re-discovery;
//  1. other built-in API groups;
//  2. the Gateway API and volume snapshot groups;
//  3. well-known operator groups (operatorGroups);
//  4. every other custom or aggregated API.
//
// Types already running are kept before new ones of any tier except
// the anchors, so a newly installed CRD never pushes out a type that is
// being watched. A type the budget leaves out is not verifiable, and a
// running type it drops stops quietly: its entities are not reported
// gone, because the objects still exist.
//
// Objects are capped too: each watched type reports at most
// DefaultMaxObjectsPerKind objects and all dynamic types together at
// most DefaultObjectBudget. Objects past a cap are cached only as small
// stubs (name, namespace, UID and resource version; see capTransform), so
// informer memory stays bounded, and produce no observations. The type is
// reported with reason object_cap_reached and a warning is logged once.
const (
	DefaultResourceBudget    = 200
	DefaultMaxObjectsPerKind = 5000
	DefaultObjectBudget      = 50000
)

// Watch tiers, see DefaultResourceBudget.
const (
	tierAnchor = iota
	tierBuiltin
	tierPreferred
	tierOperator
	tierCustom
)

// preferredGroups are custom API groups kwatch has relations for, so
// they are watched before other custom resources.
var preferredGroups = map[string]bool{
	"gateway.networking.k8s.io": true,
	"snapshot.storage.k8s.io":   true,
}

// operatorGroups are API groups of widely used operators. A group
// matches itself and its subgroups ("source.toolkit.fluxcd.io").
var operatorGroups = []string{
	"cert-manager.io", "argoproj.io", "fluxcd.io", "keda.sh",
	"karpenter.sh", "external-secrets.io",
}

func operatorGroup(group string) bool {
	for _, g := range operatorGroups {
		if group == g || strings.HasSuffix(group, "."+g) {
			return true
		}
	}
	return false
}

// metadataResources are built-in kinds watched for metadata only: their
// spec is large or churns (Lease renewals) and their status, if any,
// holds nothing detection reads.
var metadataResources = map[Resource]bool{
	{Group: "coordination.k8s.io", Name: "leases"}:                    true,
	{Group: "apps", Name: "controllerrevisions"}:                      true,
	{Group: "rbac.authorization.k8s.io", Name: "roles"}:               true,
	{Group: "rbac.authorization.k8s.io", Name: "rolebindings"}:        true,
	{Group: "rbac.authorization.k8s.io", Name: "clusterroles"}:        true,
	{Group: "rbac.authorization.k8s.io", Name: "clusterrolebindings"}: true,
}

// excludedResources are listable kinds deliberately not watched.
var excludedResources = map[Resource]bool{
	// Events are watched by the typed source as notes, in both groups.
	{Group: "events.k8s.io", Name: "events"}: true,
	// Endpoints are mirrored by the typed EndpointSlice informer and
	// change on every Pod readiness flip.
	{Name: "endpoints"}: true,
	// KwatchConfig is kwatch's own configuration; the config CRD watcher
	// already watches it and owns its restart rules.
	{Group: "kwatch.abahmed.dev", Name: "kwatchconfigs"}: true,
}

// plannedResource is one resource type the dynamic source may watch.
type plannedResource struct {
	gvr  schema.GroupVersionResource
	kind string
	mode WatchMode
	tier int
}

// typedResources are the resources the typed source already watches.
func typedResources() map[Resource]bool {
	out := map[Resource]bool{eventsResource: true}
	for _, r := range registrations() {
		out[r.resource] = true
	}
	return out
}

// planResource decides whether and how a discovered resource is watched.
// Typed and excluded kinds are skipped: a typed kind keeps its typed
// informer.
func planResource(
	gvr schema.GroupVersionResource, kind string, hasStatus bool,
	typed map[Resource]bool,
) (plannedResource, bool) {
	r := Resource{Group: gvr.Group, Name: gvr.Resource}
	if typed[r] || excludedResources[r] {
		return plannedResource{}, false
	}
	p := plannedResource{gvr: gvr, kind: kind}
	switch {
	case gvr == crdResource || gvr.GroupResource() ==
		apiServiceResource.GroupResource():
		p.tier, p.mode = tierAnchor, WatchStatus
	case builtinGroups[gvr.Group]:
		p.tier, p.mode = tierBuiltin, builtinMode(r, hasStatus)
	case preferredGroups[gvr.Group]:
		p.tier, p.mode = tierPreferred, WatchStatus
	case operatorGroup(gvr.Group):
		p.tier, p.mode = tierOperator, WatchStatus
	default:
		p.tier, p.mode = tierCustom, WatchStatus
	}
	return p, true
}

// builtinMode watches a built-in kind's status when it has a status
// subresource and is not a metadata kind.
func builtinMode(r Resource, hasStatus bool) WatchMode {
	if hasStatus && !metadataResources[r] {
		return WatchStatus
	}
	return WatchMetadata
}

// applyBudget keeps the first budget resources in budget order (see
// DefaultResourceBudget) and returns the kept and the skipped ones.
// running reports the types already watched, which stay ahead of new
// ones.
func applyBudget(
	resources []plannedResource, budget int,
	running func(schema.GroupVersionResource) bool,
	noted map[inventory.Kind]bool,
) (kept, skipped []plannedResource) {
	// Anchors first, then what is already watched, then kinds whose
	// objects had Warning events recently: over the budget, a kind
	// something complains about is worth more than one in silence.
	rank := func(r plannedResource) int {
		switch {
		case r.tier == tierAnchor:
			return 0
		case running(r.gvr):
			return 1
		case noted[KindFor(r.kind)]:
			return 2
		}
		return 3
	}
	sort.Slice(resources, func(i, j int) bool {
		a, b := resources[i], resources[j]
		if rank(a) != rank(b) {
			return rank(a) < rank(b)
		}
		if a.tier != b.tier {
			return a.tier < b.tier
		}
		if a.gvr.Group != b.gvr.Group {
			return a.gvr.Group < b.gvr.Group
		}
		return a.gvr.Resource < b.gvr.Resource
	})
	if len(resources) <= budget {
		return resources, nil
	}
	return resources[:budget], resources[budget:]
}

// auditedDynamicResources are the dynamically watched built-in types the
// RBAC audit checks: the discovery anchors, the status kinds detection
// uses and the default metadata kinds. The dynamic source watches every
// other discovered type too, but a missing grant for one only leaves it
// unwatched (DynamicStatus.Unavailable).
func auditedDynamicResources() []Resource {
	out := []Resource{
		{Group: crdResource.Group, Name: crdResource.Resource},
		{Group: apiServiceResource.Group, Name: apiServiceResource.Resource},
		{Group: "certificates.k8s.io", Name: "certificatesigningrequests"},
		{Group: "flowcontrol.apiserver.k8s.io", Name: "flowschemas"},
		{Group: "flowcontrol.apiserver.k8s.io",
			Name: "prioritylevelconfigurations"},
		{Group: admission, Name: "validatingadmissionpolicies"},
		{Group: admission, Name: "validatingadmissionpolicybindings"},
		{Group: admission, Name: "mutatingadmissionpolicies"},
		{Group: admission, Name: "mutatingadmissionpolicybindings"},
		{Group: "resource.k8s.io", Name: "resourceclaims"},
	}
	metadata := make([]Resource, 0, len(metadataResources))
	for r := range metadataResources {
		metadata = append(metadata, r)
	}
	sort.Slice(metadata, func(i, j int) bool {
		if metadata[i].Group != metadata[j].Group {
			return metadata[i].Group < metadata[j].Group
		}
		return metadata[i].Name < metadata[j].Name
	})
	return append(out, metadata...)
}
