package kube

import (
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// registration pairs a schema with its informer in the shared factory and
// the API resource the informer lists and watches.
type registration struct {
	resource Resource
	schema   Schema
	informer func(informers.SharedInformerFactory) cache.SharedIndexInformer
}

type (
	factory  = informers.SharedInformerFactory
	informer = cache.SharedIndexInformer
)

const admission = "admissionregistration.k8s.io"

// registrations lists every watched resource, grouped by area below.
func registrations() []registration {
	var all []registration
	for _, group := range [][]registration{
		workloadRegistrations(), networkRegistrations(),
		storageRegistrations(), clusterRegistrations(),
	} {
		all = append(all, group...)
	}
	return all
}

// workloadRegistrations covers pods, nodes and their controllers.
func workloadRegistrations() []registration {
	return []registration{
		{Resource{"", "pods"}, PodSchema{},
			func(f factory) informer {
				return f.Core().V1().Pods().Informer()
			}},
		{Resource{"", "nodes"}, NodeSchema{},
			func(f factory) informer {
				return f.Core().V1().Nodes().Informer()
			}},
		{Resource{"apps", "deployments"}, DeploymentSchema(),
			func(f factory) informer {
				return f.Apps().V1().Deployments().Informer()
			}},
		{Resource{"apps", "replicasets"}, ReplicaSetSchema(),
			func(f factory) informer {
				return f.Apps().V1().ReplicaSets().Informer()
			}},
		{Resource{"apps", "statefulsets"}, StatefulSetSchema(),
			func(f factory) informer {
				return f.Apps().V1().StatefulSets().Informer()
			}},
		{Resource{"apps", "daemonsets"}, DaemonSetSchema(),
			func(f factory) informer {
				return f.Apps().V1().DaemonSets().Informer()
			}},
		{Resource{"batch", "jobs"}, JobSchema(),
			func(f factory) informer {
				return f.Batch().V1().Jobs().Informer()
			}},
		{Resource{"batch", "cronjobs"}, CronJobSchema{},
			func(f factory) informer {
				return f.Batch().V1().CronJobs().Informer()
			}},
		{Resource{"autoscaling", "horizontalpodautoscalers"}, HPASchema{},
			func(f factory) informer {
				return f.Autoscaling().V2().HorizontalPodAutoscalers().
					Informer()
			}},
	}
}

// networkRegistrations covers services, routing and configuration.
func networkRegistrations() []registration {
	return []registration{
		{Resource{"", "services"}, ServiceSchema{},
			func(f factory) informer {
				return f.Core().V1().Services().Informer()
			}},
		{Resource{"discovery.k8s.io", "endpointslices"}, EndpointSliceSchema{},
			func(f factory) informer {
				return f.Discovery().V1().EndpointSlices().Informer()
			}},
		{Resource{"networking.k8s.io", "ingresses"}, IngressSchema{},
			func(f factory) informer {
				return f.Networking().V1().Ingresses().Informer()
			}},
		{SecretsResource, SecretSchema{},
			func(f factory) informer {
				return f.Core().V1().Secrets().Informer()
			}},
		{Resource{"", "configmaps"}, ConfigMapSchema{},
			func(f factory) informer {
				return f.Core().V1().ConfigMaps().Informer()
			}},
		{Resource{"", "serviceaccounts"}, ServiceAccountSchema{},
			func(f factory) informer {
				return f.Core().V1().ServiceAccounts().Informer()
			}},
	}
}

// storageRegistrations covers volumes and claims.
func storageRegistrations() []registration {
	return []registration{
		{Resource{"", "persistentvolumeclaims"}, PVCSchema{},
			func(f factory) informer {
				return f.Core().V1().PersistentVolumeClaims().Informer()
			}},
		{Resource{"", "persistentvolumes"}, PVSchema{},
			func(f factory) informer {
				return f.Core().V1().PersistentVolumes().Informer()
			}},
		{Resource{"storage.k8s.io", "storageclasses"}, StorageClassSchema{},
			func(f factory) informer {
				return f.Storage().V1().StorageClasses().Informer()
			}},
	}
}

// clusterRegistrations covers namespaces, policy and admission.
func clusterRegistrations() []registration {
	return []registration{
		{Resource{"", "namespaces"}, NamespaceSchema{},
			func(f factory) informer {
				return f.Core().V1().Namespaces().Informer()
			}},
		{Resource{"", "limitranges"}, LimitRangeSchema{},
			func(f factory) informer {
				return f.Core().V1().LimitRanges().Informer()
			}},
		{Resource{"policy", "poddisruptionbudgets"}, PDBSchema{},
			func(f factory) informer {
				return f.Policy().V1().PodDisruptionBudgets().Informer()
			}},
		{Resource{"", "resourcequotas"}, QuotaSchema{},
			func(f factory) informer {
				return f.Core().V1().ResourceQuotas().Informer()
			}},
		{Resource{"networking.k8s.io", "networkpolicies"}, NetworkPolicySchema{},
			func(f factory) informer {
				return f.Networking().V1().NetworkPolicies().Informer()
			}},
		{Resource{"storage.k8s.io", "volumeattachments"},
			VolumeAttachmentSchema{},
			func(f factory) informer {
				return f.Storage().V1().VolumeAttachments().Informer()
			}},
		{Resource{admission, "mutatingwebhookconfigurations"},
			WebhookSchema{Mutating: true},
			func(f factory) informer {
				return f.Admissionregistration().V1().
					MutatingWebhookConfigurations().Informer()
			}},
		{Resource{admission, "validatingwebhookconfigurations"},
			WebhookSchema{},
			func(f factory) informer {
				return f.Admissionregistration().V1().
					ValidatingWebhookConfigurations().Informer()
			}},
	}
}

// SecretsResource is the core Secret resource, which watch.secrets can
// turn off.
var SecretsResource = Resource{Name: "secrets"}

// hashedResources are typed kinds whose cache keeps a data hash only.
var hashedResources = map[Resource]bool{
	{Name: "secrets"}: true, {Name: "configmaps"}: true,
}
