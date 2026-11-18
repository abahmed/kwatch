package dashboard

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// frontWhy is the sentence for an object in front of the failing pods:
// one that selects, routes to or scales them, directly or through one
// Service. "" when it does none of that.
func (l *changeLinks) frontWhy(id inventory.EntityID) string {
	if l.reaches(id, inventory.Scales) {
		return "This autoscaler scales the failing workload."
	}
	if l.reaches(id, inventory.Selects) {
		return selectsWhy(id.Kind)
	}
	if l.reaches(id, inventory.RoutesTo) {
		return "This " + kindWord(id.Kind) +
			" routes traffic to the failing pods."
	}
	for _, svc := range l.targets(id, inventory.RoutesTo) {
		if svc.Kind == kube.KindService &&
			l.reaches(svc, inventory.Selects) {
			return "This " + kindWord(id.Kind) +
				" sends traffic to the failing pods through Service " +
				svc.Name + "."
		}
	}
	return ""
}

// selectsWhy names what an object that selects the failing pods does
// with them.
func selectsWhy(kind inventory.Kind) string {
	switch kind {
	case kube.KindService:
		return "This Service routes to the failing pods."
	case kube.KindNetworkPolicy:
		return "This NetworkPolicy selects the failing pods."
	case kube.KindPDB:
		return "This disruption budget covers the failing pods."
	}
	return "This " + kindWord(kind) + " selects the failing pods."
}

// reaches reports an object whose relation of type kind points at a
// failing pod or one of their workloads.
func (l *changeLinks) reaches(
	id inventory.EntityID, kind inventory.RelationType,
) bool {
	for _, target := range l.targets(id, kind) {
		if l.pods[target] || l.owners[target] || l.failing[target] {
			return true
		}
	}
	return false
}

// targets are the objects id points at with a relation of type kind:
// the stored edges, and for selects the pods its selector matches now.
func (l *changeLinks) targets(
	id inventory.EntityID, kind inventory.RelationType,
) []inventory.EntityID {
	out := l.model.Related(id, kind, inventory.Outgoing)
	if kind != inventory.Selects {
		return out
	}
	for _, link := range kube.Links(l.model, id) {
		if link.Type == inventory.Selects {
			out = append(out, link.To)
		}
	}
	return out
}

// kindWords spell the kinds a reason names the way people say them.
var kindWords = map[inventory.Kind]string{
	kube.KindConfigMap:     "ConfigMap",
	kube.KindSecret:        "Secret",
	kube.KindAccount:       "ServiceAccount",
	kube.KindPVC:           "volume claim",
	kube.KindService:       "Service",
	kube.KindIngress:       "Ingress",
	kube.KindNetworkPolicy: "NetworkPolicy",
	kube.KindHPA:           "autoscaler",
	kube.KindPDB:           "disruption budget",
	kube.KindPriorityClass: "PriorityClass",
	kube.KindRuntimeClass:  "RuntimeClass",
	kube.KindStorageClass:  "StorageClass",
	kube.KindImage:         "image",
	kube.KindNode:          "node",
}

// kindWord is the plain name of kind: its spelling above, else the
// kind itself ("httproute").
func kindWord(kind inventory.Kind) string {
	if word, ok := kindWords[kind]; ok {
		return word
	}
	return strings.ToLower(string(kind))
}
