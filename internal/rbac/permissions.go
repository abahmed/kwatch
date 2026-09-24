package rbac

import (
	"github.com/abahmed/kwatch/internal/config"
)

func permissionsForRuntime(
	runtime config.RuntimeConfig,
) ([]Permission, []Permission) {
	if !runtime.Compiled() {
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
	}
	builder.cluster = append(builder.cluster, Permission{
		Resource: "selfsubjectaccessreviews",
		Group:    "authorization.k8s.io",
		Verb:     "create",
	})
	addWorkloadPermissions(&builder, runtime)
	addNetworkPermissions(&builder, runtime)
	addNodeStoragePermissions(&builder, runtime)
	addControlPlaneSecurityPermissions(&builder, runtime)
	addClusterResourcePermissions(&builder, runtime)
	if runtime.Monitors().IncludeLogs() {
		builder.namespaced = append(builder.namespaced, Permission{
			Resource: "pods/log",
			Verb:     "get",
		})
	}
	// The CRD watcher is startup/live infrastructure and is independent of
	// workload monitors; it must retain access when the installed CRD exists.
	builder.addCluster(
		runtime.Monitors().CRD().Enabled,
		Permission{
			Resource: "customresourcedefinitions",
			Group:    "apiextensions.k8s.io",
		},
	)
	return deduplicate(builder.cluster), deduplicate(builder.namespaced)
}

func infrastructurePermissionsForRuntime(
	runtime config.RuntimeConfig,
) []Permission {
	permissions := []Permission{{Resource: "configmaps", Verb: "create"}}
	for _, name := range persistenceConfigMapNames() {
		for _, verb := range []string{"get", "update", "patch"} {
			permissions = append(permissions, Permission{
				Name:     name,
				Resource: "configmaps",
				Verb:     verb,
			})
		}
	}
	if runtime.Monitors().CRD().Enabled {
		permissions = append(permissions, permissionResources(Permission{
			Resource: "kwatchconfigs",
			Group:    "kwatch.abahmed.dev",
		})...)
	}
	return permissions
}

func persistenceConfigMapNames() []string {
	return []string{
		"kwatch-state",
		"kwatch-baseline",
		"kwatch-incidents",
		"kwatch-groups",
		"kwatch-threads",
		"kwatch-engine",
		"kwatch-pvc",
		"kwatch-changes",
		"kwatch-rca",
		"kwatch-telemetry",
	}
}

type permissionBuilder struct {
	cluster, namespaced []Permission
}

func (b *permissionBuilder) addNamespaced(
	enabled bool, resources ...Permission,
) {
	if enabled {
		b.namespaced = append(b.namespaced, permissionResources(resources...)...)
	}
}

func (b *permissionBuilder) addCluster(enabled bool, resources ...Permission) {
	if enabled {
		b.cluster = append(b.cluster, permissionResources(resources...)...)
	}
}

func addWorkloadPermissions(
	b *permissionBuilder, runtime config.RuntimeConfig,
) {
	b.addNamespaced(runtime.Monitors().Rollout().Enabled, apps("deployments")...)
	b.addNamespaced(runtime.Monitors().Rollout().Enabled, apps("replicasets")...)
	b.addNamespaced(
		runtime.Monitors().StatefulSet().Enabled, apps("statefulsets")...,
	)
	b.addNamespaced(runtime.Monitors().DaemonSet().Enabled, apps("daemonsets")...)
	b.addNamespaced(runtime.Monitors().Job().Enabled, batch("jobs")...)
	b.addNamespaced(runtime.Monitors().CronJob().Enabled, batch("cronjobs")...)
	b.addNamespaced(
		runtime.Monitors().HPA().Enabled,
		autoscaling("horizontalpodautoscalers")...,
	)
	b.addNamespaced(
		runtime.Monitors().PDB().Enabled,
		policy("poddisruptionbudgets")...,
	)
}

func addNetworkPermissions(
	b *permissionBuilder, runtime config.RuntimeConfig,
) {
	service := runtime.Monitors().Service().Enabled ||
		(runtime.Monitors().ActiveProbe().Enabled &&
			runtime.Monitors().ActiveProbe().AutoServices)
	ingress := runtime.Monitors().Ingress().Enabled
	b.addNamespaced(
		service,
		Permission{Resource: "services"},
		Permission{Resource: "endpointslices", Group: "discovery.k8s.io"},
	)
	b.addNamespaced(
		ingress || runtime.Monitors().AdmissionWebhook().Enabled,
		Permission{Resource: "services"},
	)
	b.addNamespaced(
		service || ingress || runtime.Monitors().AdmissionWebhook().Enabled,
		Permission{Resource: "endpointslices", Group: "discovery.k8s.io"},
	)
	b.addNamespaced(ingress, networking("ingresses")...)
	b.addNamespaced(
		runtime.Monitors().NetworkPolicy().Enabled,
		networking("networkpolicies")...,
	)
	b.addNamespaced(
		runtime.Monitors().TLS().Enabled,
		Permission{Resource: "secrets"},
	)
}

func addNodeStoragePermissions(
	b *permissionBuilder, runtime config.RuntimeConfig,
) {
	// The PVC monitor reads volume usage from each kubelet's summary
	// endpoint, so it needs nodes and nodes/proxy just as the node monitors
	// do. Leaving it out of this set meant a PVC-only configuration reported
	// full RBAC while every usage sweep was being denied.
	nodes := runtime.Monitors().Node().Enabled ||
		runtime.Monitors().NodeResource().Enabled ||
		runtime.Monitors().KubeletTelemetry().Enabled ||
		runtime.Monitors().ControlPlane().Enabled || runtime.Monitors().PVC().Enabled
	b.addCluster(
		nodes, Permission{Resource: "nodes"}, Permission{Resource: "nodes/proxy"},
	)
	b.addCluster(runtime.Monitors().PVC().Enabled,
		Permission{Resource: "persistentvolumes"},
		Permission{Resource: "storageclasses", Group: "storage.k8s.io"},
		Permission{Resource: "volumeattachments", Group: "storage.k8s.io"})
}

func addControlPlaneSecurityPermissions(
	b *permissionBuilder, runtime config.RuntimeConfig,
) {
	b.addCluster(
		runtime.Monitors().ControlPlane().Enabled,
		Permission{Resource: "pods"},
		Permission{Resource: "pods/proxy"},
	)
	if runtime.Monitors().ControlPlane().Enabled {
		b.cluster = append(b.cluster, Permission{
			NonResourceURL: "/readyz",
			Verb:           "get",
		})
	}
	b.addCluster(
		runtime.Monitors().ControlPlane().Enabled,
		Permission{Resource: "apiservices", Group: "apiregistration.k8s.io"},
	)
	b.addCluster(runtime.Monitors().AdmissionWebhook().Enabled,
		Permission{
			Resource: "mutatingwebhookconfigurations",
			Group:    "admissionregistration.k8s.io",
		},
		Permission{
			Resource: "validatingwebhookconfigurations",
			Group:    "admissionregistration.k8s.io",
		},
	)
}

func addClusterResourcePermissions(
	b *permissionBuilder, runtime config.RuntimeConfig,
) {
	if !runtime.Monitors().ClusterResource().Enabled {
		return
	}
	b.addNamespaced(
		true,
		Permission{Resource: "resourcequotas"},
		Permission{Resource: "limitranges"},
		Permission{Resource: "endpoints"},
		Permission{
			Resource: "podcertificaterequests",
			Group:    "certificates.k8s.io",
		},
		Permission{
			Resource: "volumesnapshots",
			Group:    "snapshot.storage.k8s.io",
		},
		Permission{
			Resource: "gateways",
			Group:    "gateway.networking.k8s.io",
		},
		Permission{
			Resource: "httproutes",
			Group:    "gateway.networking.k8s.io",
		},
		Permission{
			Resource: "grpcroutes",
			Group:    "gateway.networking.k8s.io",
		},
		Permission{
			Resource: "tcproutes",
			Group:    "gateway.networking.k8s.io",
		},
		Permission{
			Resource: "tlsroutes",
			Group:    "gateway.networking.k8s.io",
		},
		Permission{
			Resource: "referencegrants",
			Group:    "gateway.networking.k8s.io",
		},
	)
	b.addCluster(
		true,
		Permission{Resource: "namespaces"},
		Permission{
			Resource: "apiservices",
			Group:    "apiregistration.k8s.io",
		},
		Permission{
			Resource: "validatingadmissionpolicies",
			Group:    "admissionregistration.k8s.io",
		},
		Permission{
			Resource: "validatingadmissionpolicybindings",
			Group:    "admissionregistration.k8s.io",
		},
		Permission{
			Resource: "mutatingadmissionpolicies",
			Group:    "admissionregistration.k8s.io",
		},
		Permission{
			Resource: "mutatingadmissionpolicybindings",
			Group:    "admissionregistration.k8s.io",
		},
		Permission{
			Resource: "certificatesigningrequests",
			Group:    "certificates.k8s.io",
		},
		Permission{
			Resource: "flowschemas",
			Group:    "flowcontrol.apiserver.k8s.io",
		},
		Permission{
			Resource: "prioritylevelconfigurations",
			Group:    "flowcontrol.apiserver.k8s.io",
		},
		Permission{
			Resource: "volumeattachments",
			Group:    "storage.k8s.io",
		},
		Permission{Resource: "csidrivers", Group: "storage.k8s.io"},
		Permission{
			Resource: "volumesnapshotcontents",
			Group:    "snapshot.storage.k8s.io",
		},
		Permission{
			Resource: "volumesnapshotclasses",
			Group:    "snapshot.storage.k8s.io",
		},
		Permission{
			Resource: "gatewayclasses",
			Group:    "gateway.networking.k8s.io",
		},
	)
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

func apps(resource string) []Permission {
	return []Permission{{Resource: resource, Group: "apps"}}
}

func batch(resource string) []Permission {
	return []Permission{{Resource: resource, Group: "batch"}}
}

func autoscaling(resource string) []Permission {
	return []Permission{{Resource: resource, Group: "autoscaling"}}
}

func policy(resource string) []Permission {
	return []Permission{{Resource: resource, Group: "policy"}}
}

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
