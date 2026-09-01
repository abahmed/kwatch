package security

import (
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
)

// permissionsForConfig mirrors controller wiring: a disabled monitor should
// not make the security report ask for access to its resource. Pods and Events
// remain the base capability because the core pod pipeline is always active.
func permissionsForConfig(cfg *config.Config) ([]Permission, []Permission) {
	return permissionsForConfigAndPlan(cfg, feature.Plan{})
}

func permissionsForConfigAndPlan(cfg *config.Config, plan feature.Plan) ([]Permission, []Permission) {
	if cfg == nil {
		return clusterPermissions(), namespacedPermissions()
	}
	// These resources are part of the controller's baseline graph and owner
	// resolution wiring, even when their dedicated detector is disabled.
	builder := permissionBuilder{
		cluster: permissionResources(
			Permission{Resource: "leases", Group: "coordination.k8s.io"},
			Permission{Resource: "persistentvolumes"},
			Permission{Resource: "serviceaccounts"},
			Permission{Resource: "storageclasses", Group: "storage.k8s.io"},
		),
		namespaced: permissionResources(
			Permission{Resource: "pods"},
			Permission{Resource: "events"},
			Permission{Resource: "configmaps"},
			Permission{Resource: "secrets"},
			Permission{Resource: "persistentvolumeclaims"},
			Permission{Resource: "replicasets", Group: "apps"},
			Permission{Resource: "statefulsets", Group: "apps"},
			Permission{Resource: "daemonsets", Group: "apps"},
		),
		plan: plan,
	}
	builder.cluster = append(builder.cluster, Permission{
		Resource: "selfsubjectaccessreviews", Group: "authorization.k8s.io", Verb: "create",
	})
	addWorkloadPermissions(&builder, cfg)
	addNetworkPermissions(&builder, cfg)
	addNodeStoragePermissions(&builder, cfg)
	addControlPlaneSecurityPermissions(&builder, cfg)
	addClusterResourcePermissions(&builder, cfg)
	// The CRD watcher is startup/live infrastructure and is independent of
	// workload monitors; it must retain access when the installed CRD exists.
	builder.addCluster(cfg.CrdConfig.Enabled && planAllows(plan, feature.CRDDiscovery), Permission{Resource: "customresourcedefinitions", Group: "apiextensions.k8s.io"})
	return deduplicate(builder.cluster), deduplicate(builder.namespaced)
}

type permissionBuilder struct {
	cluster, namespaced []Permission
	plan                feature.Plan
}

func (b *permissionBuilder) addNamespaced(enabled bool, resources ...Permission) {
	if enabled {
		b.namespaced = append(b.namespaced, permissionResources(resources...)...)
	}
}

func (b *permissionBuilder) addCluster(enabled bool, resources ...Permission) {
	if enabled {
		b.cluster = append(b.cluster, permissionResources(resources...)...)
	}
}

func addWorkloadPermissions(b *permissionBuilder, cfg *config.Config) {
	b.addNamespaced(cfg.RolloutMonitor.Enabled && planAllows(b.plan, feature.DeploymentRollout), apps("deployments")...)
	b.addNamespaced(cfg.RolloutMonitor.Enabled && planAllows(b.plan, feature.DeploymentRollout), apps("replicasets")...)
	b.addNamespaced(cfg.StatefulSetMonitor.Enabled && planAllows(b.plan, feature.StatefulSetRollout), apps("statefulsets")...)
	b.addNamespaced(cfg.DaemonSetMonitor.Enabled && planAllows(b.plan, feature.DaemonSetRollout), apps("daemonsets")...)
	b.addNamespaced(cfg.JobMonitor.Enabled && planAllows(b.plan, feature.JobFailures), batch("jobs")...)
	b.addNamespaced(cfg.CronJobMonitor.Enabled && planAllows(b.plan, feature.CronJobFailures), batch("cronjobs")...)
	b.addNamespaced(cfg.HpaMonitor.Enabled && planAllows(b.plan, feature.HPADiagnostics), autoscaling("horizontalpodautoscalers")...)
	b.addNamespaced(cfg.PdbMonitor.Enabled && planAllows(b.plan, feature.PDBViolations), policy("poddisruptionbudgets")...)
}

func addNetworkPermissions(b *permissionBuilder, cfg *config.Config) {
	service := cfg.ServiceMonitor.Enabled && planAllows(b.plan, feature.ServiceEndpoints)
	ingress := cfg.IngressMonitor.Enabled && planAllows(b.plan, feature.IngressBackends)
	b.addNamespaced(service, Permission{Resource: "services"}, Permission{Resource: "endpointslices", Group: "discovery.k8s.io"})
	b.addNamespaced(ingress || cfg.AdmissionWebhookMonitor.Enabled, Permission{Resource: "services"})
	b.addNamespaced(service || ingress || cfg.AdmissionWebhookMonitor.Enabled, Permission{Resource: "endpointslices", Group: "discovery.k8s.io"})
	b.addNamespaced(ingress, networking("ingresses")...)
	b.addNamespaced(cfg.NetworkPolicyMonitor.Enabled && planAllows(b.plan, feature.NetworkPolicies), networking("networkpolicies")...)
	b.addNamespaced(cfg.TlsMonitor.Enabled && planAllows(b.plan, feature.TLSMonitoring), Permission{Resource: "secrets"})
}

func addNodeStoragePermissions(b *permissionBuilder, cfg *config.Config) {
	nodes := (cfg.NodeMonitor.Enabled && planAllows(b.plan, feature.NodeConditions)) ||
		(cfg.NodeResourceMonitor.Enabled && planAllows(b.plan, feature.NodeResources)) ||
		(cfg.KubeletTelemetryMonitor.Enabled && planAllows(b.plan, feature.KubeletTelemetry)) ||
		(cfg.ControlPlaneMonitor.Enabled && planAllows(b.plan, feature.ControlPlanePods))
	b.addCluster(nodes, Permission{Resource: "nodes"}, Permission{Resource: "nodes/proxy"})
	b.addCluster(cfg.PvcMonitor.Enabled && planAllows(b.plan, feature.PVCUsage),
		Permission{Resource: "persistentvolumes"}, Permission{Resource: "storageclasses", Group: "storage.k8s.io"},
		Permission{Resource: "volumeattachments", Group: "storage.k8s.io"})
}

func addControlPlaneSecurityPermissions(b *permissionBuilder, cfg *config.Config) {
	b.addCluster(cfg.ControlPlaneMonitor.Enabled && planAllows(b.plan, feature.APIServerHealth), Permission{Resource: "apiservices", Group: "apiregistration.k8s.io"})
	b.addCluster(cfg.AdmissionWebhookMonitor.Enabled && planAllows(b.plan, feature.AdmissionWebhooks),
		Permission{Resource: "mutatingwebhookconfigurations", Group: "admissionregistration.k8s.io"},
		Permission{Resource: "validatingwebhookconfigurations", Group: "admissionregistration.k8s.io"})
}

func addClusterResourcePermissions(b *permissionBuilder, cfg *config.Config) {
	if !cfg.ClusterResourceMonitor.Enabled || !planAllows(b.plan, feature.ClusterResources) {
		return
	}
	b.addNamespaced(true, Permission{Resource: "resourcequotas"}, Permission{Resource: "limitranges"})
	b.addCluster(true, Permission{Resource: "namespaces"}, Permission{Resource: "apiservices", Group: "apiregistration.k8s.io"},
		Permission{Resource: "validatingadmissionpolicies", Group: "admissionregistration.k8s.io"},
		Permission{Resource: "validatingadmissionpolicybindings", Group: "admissionregistration.k8s.io"},
		Permission{Resource: "mutatingadmissionpolicies", Group: "admissionregistration.k8s.io"},
		Permission{Resource: "mutatingadmissionpolicybindings", Group: "admissionregistration.k8s.io"},
		Permission{Resource: "certificatesigningrequests", Group: "certificates.k8s.io"}, Permission{Resource: "podcertificaterequests", Group: "certificates.k8s.io"},
		Permission{Resource: "flowschemas", Group: "flowcontrol.apiserver.k8s.io"}, Permission{Resource: "prioritylevelconfigurations", Group: "flowcontrol.apiserver.k8s.io"},
		Permission{Resource: "endpoints"}, Permission{Resource: "volumeattachments", Group: "storage.k8s.io"}, Permission{Resource: "csidrivers", Group: "storage.k8s.io"},
		Permission{Resource: "volumesnapshots", Group: "snapshot.storage.k8s.io"}, Permission{Resource: "volumesnapshotcontents", Group: "snapshot.storage.k8s.io"}, Permission{Resource: "volumesnapshotclasses", Group: "snapshot.storage.k8s.io"},
		Permission{Resource: "gatewayclasses", Group: "gateway.networking.k8s.io"}, Permission{Resource: "gateways", Group: "gateway.networking.k8s.io"}, Permission{Resource: "httproutes", Group: "gateway.networking.k8s.io"},
		Permission{Resource: "grpcroutes", Group: "gateway.networking.k8s.io"}, Permission{Resource: "tcproutes", Group: "gateway.networking.k8s.io"}, Permission{Resource: "tlsroutes", Group: "gateway.networking.k8s.io"}, Permission{Resource: "referencegrants", Group: "gateway.networking.k8s.io"})
}

func planAllows(plan feature.Plan, id feature.ID) bool {
	return len(plan.Decisions) == 0 || plan.Enabled(id)
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
