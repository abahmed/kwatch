package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// kindWords are the words people use for kinds whose API name reads
// badly in a sentence. Acronyms keep their capitals.
var kindWords = map[inventory.Kind]string{
	kube.KindPVC:                 "volume claim",
	kube.KindPV:                  "volume",
	kube.KindHPA:                 "autoscaler",
	kube.KindAccount:             "service account",
	kube.KindConfigMap:           "config map",
	kube.KindPDB:                 "disruption budget",
	kube.KindNetworkPolicy:       "network policy",
	kube.KindQuota:               "quota",
	kube.KindStorageClass:        "storage class",
	kube.KindCronJob:             "cron job",
	kube.KindReplicaSet:          "replica set",
	kube.KindStatefulSet:         "stateful set",
	kube.KindDaemonSet:           "daemon set",
	kube.KindEndpointSlice:       "endpoint slice",
	kube.KindAPIService:          "API service",
	kube.KindCRD:                 "CRD",
	kube.KindValidatingHook:      "validating webhook",
	kube.KindMutatingWebhook:     "mutating webhook",
	kube.KindIngressClass:        "ingress class",
	kube.KindPriorityClass:       "priority class",
	kube.KindRuntimeClass:        "runtime class",
	kube.KindLimitRange:          "limit range",
	kube.KindNodePool:            "node pool",
	"rolebinding":                "role binding",
	"clusterrolebinding":         "cluster role binding",
	"clusterrole":                "cluster role",
	"httproute":                  "HTTP route",
	"grpcroute":                  "gRPC route",
	"tlsroute":                   "TLS route",
	"tcproute":                   "TCP route",
	"cluster-dns":                "cluster DNS",
	"apiserver":                  "API server",
	"controller-manager":         "controller manager",
	rootcause.KindScheduling:     "scheduling",
	explain.KindExternalEndpoint: "external endpoint",
}

// singletonNames name the cluster services there is only one of: their
// object name adds nothing ("cluster DNS", not "cluster-dns
// cluster-dns").
var singletonNames = map[inventory.EntityID]string{
	kube.ClusterDNS:        "cluster DNS",
	kube.APIServer:         "the Kubernetes API",
	kube.Etcd:              "etcd",
	kube.Scheduler:         "the scheduler",
	kube.ControllerManager: "the controller manager",
	kube.KwatchSelf:        "kwatch",
}

func kindWord(kind inventory.Kind) string {
	if word, ok := kindWords[kind]; ok {
		return word
	}
	return string(kind)
}

// shortName is how a note names an entity: a workload by its own name,
// anything else with its kind ("node n1", "service cart").
func shortName(id inventory.EntityID) string {
	if name, ok := singletonNames[id]; ok {
		return name
	}
	switch {
	case incident.IsWorkload(id.Kind):
		return id.Name
	case id.Kind == kube.KindContainer:
		pod, container := splitContainer(id.Name)
		return "container " + container + " in pod " + pod
	case id.Kind == explain.KindFailureSignature:
		return "the error \"" + explain.SignatureText(id.Name) + "\""
	}
	return kindWord(id.Kind) + " " + id.Name
}

// placedName adds the namespace to shortName ("payments in shop").
func placedName(id inventory.EntityID) string {
	if id.Namespace == "" {
		return shortName(id)
	}
	return shortName(id) + " in " + id.Namespace
}

// nameFrom names other relative to home: the namespace is only said
// when it differs.
func nameFrom(home, other inventory.EntityID) string {
	if other.Namespace == home.Namespace {
		return shortName(other)
	}
	return placedName(other)
}

// startsWithName reports whether a sentence about id starts with the
// resource's own name, which must keep its case ("payments", "etcd").
// Every other name starts with a kind word that is capitalised.
func startsWithName(id inventory.EntityID) bool {
	return incident.IsWorkload(id.Kind) || id == kube.Etcd
}

// capitalName capitalises text, a sentence that starts with id's name,
// unless the name must keep its case.
func capitalName(id inventory.EntityID, text string) string {
	if startsWithName(id) {
		return text
	}
	return upperFirst(text)
}

// capitalKind capitalises a sentence that starts with a kind word
// ("service cart …" becomes "Service cart …"). A sentence that starts
// with a resource's own name keeps it exactly as written.
func capitalKind(text string) string {
	first, _, _ := strings.Cut(text, " ")
	if sentenceKinds[first] {
		return upperFirst(text)
	}
	return text
}

// sentenceKinds are the first words of every kind name used in notes.
var sentenceKinds = func() map[string]bool {
	out := map[string]bool{"container": true, "pod": true, "node": true,
		"service": true, "ingress": true, "secret": true, "job": true,
		"namespace": true, "registry": true, "zone": true, "the": true}
	for _, word := range kindWords {
		first, _, _ := strings.Cut(word, " ")
		out[first] = true
	}
	return out
}()
