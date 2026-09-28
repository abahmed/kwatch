package kube

import (
	"strings"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// Entity kinds produced by this package.
const (
	KindPod           knowledge.Kind = "pod"
	KindContainer     knowledge.Kind = "container"
	KindNode          knowledge.Kind = "node"
	KindDeployment    knowledge.Kind = "deployment"
	KindReplicaSet    knowledge.Kind = "replicaset"
	KindStatefulSet   knowledge.Kind = "statefulset"
	KindDaemonSet     knowledge.Kind = "daemonset"
	KindJob           knowledge.Kind = "job"
	KindCronJob       knowledge.Kind = "cronjob"
	KindHPA           knowledge.Kind = "horizontalpodautoscaler"
	KindService       knowledge.Kind = "service"
	KindEndpointSlice knowledge.Kind = "endpointslice"
	KindIngress       knowledge.Kind = "ingress"
	KindSecret        knowledge.Kind = "secret"
	KindConfigMap     knowledge.Kind = "configmap"
	KindAccount       knowledge.Kind = "serviceaccount"
	KindPVC           knowledge.Kind = "persistentvolumeclaim"
	KindPV            knowledge.Kind = "persistentvolume"
	KindStorageClass  knowledge.Kind = "storageclass"
	KindPriorityClass knowledge.Kind = "priorityclass"
	KindRuntimeClass  knowledge.Kind = "runtimeclass"
	KindNamespace     knowledge.Kind = "namespace"
	KindImage         knowledge.Kind = "image"
	KindRegistry      knowledge.Kind = "registry"
	KindZone          knowledge.Kind = "zone"
	KindNodePool      knowledge.Kind = "nodepool"
)

// KindFor maps a Kubernetes Kind ("ReplicaSet") to an entity kind.
func KindFor(kubernetesKind string) knowledge.Kind {
	return knowledge.Kind(strings.ToLower(kubernetesKind))
}
