package controller

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

// Edge types added by the per-resource graph builders. Keep them stable: RCA
// and graph diagnostics use these values as relationship vocabulary.
const (
	graphEdgeRoutesTo  = "routes_to"
	graphEdgeScales    = "scales"
	graphEdgeAppliesTo = "applies_to"
	graphEdgeBinds     = "binds"
	graphEdgeBacks     = "backs"
	graphEdgeTargets   = "targets"
	graphEdgeProvides  = "provides"
	graphEdgeUnready   = "unready"
	graphEdgeUsesSA    = "uses_sa"
	graphEdgeUsesPull  = "uses_pull_secret"
	graphEdgeProjects  = "projects"
	graphEdgeUsesCSI   = "uses_csi"
	graphEdgeUsesSC    = "uses_sc"
	graphEdgeLocalAt   = "local_at"
	graphEdgeTLS       = "tls_secret"
	graphEdgeProtects  = "protects"
)

// graphHandler keeps a resource node current as informer objects change.
func (c *Controller) graphHandler(
	kind string,
	rebuild func(interface{}),
) cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { rebuild(obj) },
		UpdateFunc: func(_, newObj interface{}) { rebuild(newObj) },
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err != nil {
				return
			}
			ns, name, _ := cache.SplitMetaNamespaceKey(key)
			if c.graph != nil {
				c.graph.RemoveNode(kind, ns, name)
			}
		},
	}
}

// ownedByTargets completes owner chains the Pod informer cannot see directly.
func ownedByTargets(
	namespace string,
	ownerRefs []metav1.OwnerReference,
) []kwcontext.EdgeTarget {
	targets := make([]kwcontext.EdgeTarget, 0, len(ownerRefs))
	for _, ref := range ownerRefs {
		kind := strings.ToLower(ref.Kind)
		if !isTrackedWorkload(kind) {
			continue
		}
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: kind, Namespace: namespace,
			Name: ref.Name, Type: "owned_by",
		})
	}
	return targets
}

func isTrackedWorkload(kind string) bool {
	switch kind {
	case "deployment", "statefulset", "daemonset", "replicaset", "job", "cronjob":
		return true
	default:
		return false
	}
}
