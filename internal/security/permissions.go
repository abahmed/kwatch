package security

import "github.com/abahmed/kwatch/internal/config"

// permissionsForConfig mirrors controller wiring: a disabled monitor should
// not make the security report ask for access to its resource. Pods and Events
// remain the base capability because the core pod pipeline is always active.
func permissionsForConfig(cfg *config.Config) ([]Permission, []Permission) {
	if cfg == nil {
		return clusterPermissions(), namespacedPermissions()
	}
	cluster := permissionResources(
		Permission{Resource: "leases", Group: "coordination.k8s.io"},
	)
	namespaced := permissionResources(
		Permission{Resource: "pods"},
		Permission{Resource: "events"},
	)
	addNamespaced := func(enabled bool, resources ...Permission) {
		if enabled {
			namespaced = append(namespaced, permissionResources(resources...)...)
		}
	}
	addCluster := func(enabled bool, resources ...Permission) {
		if enabled {
			cluster = append(cluster, permissionResources(resources...)...)
		}
	}

	addNamespaced(cfg.RolloutMonitor.Enabled, apps("deployments")...)
	addNamespaced(cfg.RolloutMonitor.Enabled, apps("replicasets")...)
	addNamespaced(cfg.StatefulSetMonitor.Enabled, apps("statefulsets")...)
	addNamespaced(cfg.DaemonSetMonitor.Enabled, apps("daemonsets")...)
	addNamespaced(cfg.JobMonitor.Enabled, batch("jobs")...)
	addNamespaced(cfg.CronJobMonitor.Enabled, batch("cronjobs")...)
	addNamespaced(cfg.HpaMonitor.Enabled, autoscaling("horizontalpodautoscalers")...)
	addNamespaced(cfg.PdbMonitor.Enabled, policy("poddisruptionbudgets")...)
	addNamespaced(cfg.ServiceMonitor.Enabled, Permission{Resource: "services"}, Permission{Resource: "endpointslices", Group: "discovery.k8s.io"})
	addNamespaced(cfg.IngressMonitor.Enabled, networking("ingresses")...)
	addNamespaced(cfg.NetworkPolicyMonitor.Enabled, networking("networkpolicies")...)
	addNamespaced(cfg.ClusterResourceMonitor.Enabled, Permission{Resource: "resourcequotas"}, Permission{Resource: "limitranges"})
	addNamespaced(cfg.TlsMonitor.Enabled, Permission{Resource: "secrets"})

	nodesNeeded := cfg.NodeMonitor.Enabled || cfg.NodeResourceMonitor.Enabled || cfg.KubeletTelemetryMonitor.Enabled || cfg.ControlPlaneMonitor.Enabled
	addCluster(nodesNeeded, Permission{Resource: "nodes"})
	addCluster(cfg.ClusterResourceMonitor.Enabled, Permission{Resource: "namespaces"})
	addCluster(cfg.PvcMonitor.Enabled,
		Permission{Resource: "persistentvolumes"},
		Permission{Resource: "storageclasses", Group: "storage.k8s.io"},
		Permission{Resource: "volumeattachments", Group: "storage.k8s.io"},
	)
	addCluster(cfg.ControlPlaneMonitor.Enabled, Permission{Resource: "apiservices", Group: "apiregistration.k8s.io"})
	addCluster(cfg.AdmissionWebhookMonitor.Enabled,
		Permission{Resource: "mutatingwebhookconfigurations", Group: "admissionregistration.k8s.io"},
		Permission{Resource: "validatingwebhookconfigurations", Group: "admissionregistration.k8s.io"},
	)
	// The CRD watcher is startup/live infrastructure and is independent of
	// workload monitors; it must retain access when the installed CRD exists.
	addCluster(true, Permission{Resource: "customresourcedefinitions", Group: "apiextensions.k8s.io"})
	return deduplicate(cluster), deduplicate(namespaced)
}

func permissionResources(resources ...Permission) []Permission {
	permissions := make([]Permission, 0, len(resources)*3)
	for _, resource := range resources {
		for _, verb := range []string{"get", "list", "watch"} {
			resource.Verb = verb
			permissions = append(permissions, resource)
		}
	}
	return permissions
}

func apps(resource string) []Permission { return []Permission{{Resource: resource, Group: "apps"}} }

func batch(resource string) []Permission { return []Permission{{Resource: resource, Group: "batch"}} }

func autoscaling(resource string) []Permission {
	return []Permission{{Resource: resource, Group: "autoscaling"}}
}

func policy(resource string) []Permission { return []Permission{{Resource: resource, Group: "policy"}} }

func networking(resource string) []Permission {
	return []Permission{{Resource: resource, Group: "networking.k8s.io"}}
}

func deduplicate(permissions []Permission) []Permission {
	seen := make(map[Permission]struct{}, len(permissions))
	out := make([]Permission, 0, len(permissions))
	for _, permission := range permissions {
		if _, ok := seen[permission]; ok {
			continue
		}
		seen[permission] = struct{}{}
		out = append(out, permission)
	}
	return out
}
