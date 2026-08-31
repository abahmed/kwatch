package security

import "github.com/abahmed/kwatch/internal/config"

// permissionsForConfig mirrors controller wiring: a disabled monitor should
// not make the security report ask for access to its resource. Pods and Events
// remain the base capability because the core pod pipeline is always active.
func permissionsForConfig(cfg *config.Config) ([]Permission, []Permission) {
	if cfg == nil {
		return clusterPermissions(), namespacedPermissions()
	}
	// These resources are part of the controller's baseline graph and owner
	// resolution wiring, even when their dedicated detector is disabled.
	cluster := permissionResources(
		Permission{Resource: "leases", Group: "coordination.k8s.io"},
		Permission{Resource: "persistentvolumes"},
		Permission{Resource: "serviceaccounts"},
		Permission{Resource: "storageclasses", Group: "storage.k8s.io"},
	)
	cluster = append(cluster, Permission{
		Resource: "selfsubjectaccessreviews", Group: "authorization.k8s.io", Verb: "create",
	})
	namespaced := permissionResources(
		Permission{Resource: "pods"},
		Permission{Resource: "events"},
		Permission{Resource: "configmaps"},
		Permission{Resource: "secrets"},
		Permission{Resource: "persistentvolumeclaims"},
		Permission{Resource: "replicasets", Group: "apps"},
		Permission{Resource: "statefulsets", Group: "apps"},
		Permission{Resource: "daemonsets", Group: "apps"},
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
	addNamespaced(cfg.IngressMonitor.Enabled || cfg.AdmissionWebhookMonitor.Enabled, Permission{Resource: "services"})
	addNamespaced(cfg.ServiceMonitor.Enabled || cfg.IngressMonitor.Enabled || cfg.AdmissionWebhookMonitor.Enabled,
		Permission{Resource: "endpointslices", Group: "discovery.k8s.io"})
	addNamespaced(cfg.IngressMonitor.Enabled, networking("ingresses")...)
	addNamespaced(cfg.NetworkPolicyMonitor.Enabled, networking("networkpolicies")...)
	addNamespaced(cfg.ClusterResourceMonitor.Enabled, Permission{Resource: "resourcequotas"}, Permission{Resource: "limitranges"})
	addNamespaced(cfg.TlsMonitor.Enabled, Permission{Resource: "secrets"})

	nodesNeeded := cfg.NodeMonitor.Enabled || cfg.NodeResourceMonitor.Enabled || cfg.KubeletTelemetryMonitor.Enabled || cfg.ControlPlaneMonitor.Enabled
	addCluster(nodesNeeded, Permission{Resource: "nodes"}, Permission{Resource: "nodes/proxy"})
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
	if cfg.ClusterResourceMonitor.Enabled {
		addCluster(true,
			Permission{Resource: "apiservices", Group: "apiregistration.k8s.io"},
			Permission{Resource: "validatingadmissionpolicies", Group: "admissionregistration.k8s.io"},
			Permission{Resource: "validatingadmissionpolicybindings", Group: "admissionregistration.k8s.io"},
			Permission{Resource: "mutatingadmissionpolicies", Group: "admissionregistration.k8s.io"},
			Permission{Resource: "mutatingadmissionpolicybindings", Group: "admissionregistration.k8s.io"},
			Permission{Resource: "certificatesigningrequests", Group: "certificates.k8s.io"},
			Permission{Resource: "podcertificaterequests", Group: "certificates.k8s.io"},
			Permission{Resource: "flowschemas", Group: "flowcontrol.apiserver.k8s.io"},
			Permission{Resource: "prioritylevelconfigurations", Group: "flowcontrol.apiserver.k8s.io"},
			Permission{Resource: "endpoints"},
			Permission{Resource: "volumeattachments", Group: "storage.k8s.io"},
			Permission{Resource: "csidrivers", Group: "storage.k8s.io"},
			Permission{Resource: "volumesnapshots", Group: "snapshot.storage.k8s.io"},
			Permission{Resource: "volumesnapshotcontents", Group: "snapshot.storage.k8s.io"},
			Permission{Resource: "volumesnapshotclasses", Group: "snapshot.storage.k8s.io"},
			Permission{Resource: "gatewayclasses", Group: "gateway.networking.k8s.io"},
			Permission{Resource: "gateways", Group: "gateway.networking.k8s.io"},
			Permission{Resource: "httproutes", Group: "gateway.networking.k8s.io"},
			Permission{Resource: "grpcroutes", Group: "gateway.networking.k8s.io"},
			Permission{Resource: "tcproutes", Group: "gateway.networking.k8s.io"},
			Permission{Resource: "tlsroutes", Group: "gateway.networking.k8s.io"},
			Permission{Resource: "referencegrants", Group: "gateway.networking.k8s.io"},
		)
	}
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
