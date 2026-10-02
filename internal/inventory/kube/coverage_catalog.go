package kube

// CoverageModeExcluded marks a listable kind that is deliberately not
// watched.
const CoverageModeExcluded WatchMode = "excluded"

// CoverageEntry is one built-in or well-known kind and the watch mode the
// watch plan must assign it. The catalog is checked against the plan so a
// kind cannot be dropped silently.
type CoverageEntry struct {
	Group    string
	Resource string
	Kind     string
	// Mode is the expected watch mode, or CoverageModeExcluded.
	Mode WatchMode
	// HasStatus is the discovery fixture: whether the API serves a
	// status subresource for the kind (Kubernetes v1.35/v1.36).
	HasStatus bool
}

func typedEntry(g, r, k string) CoverageEntry {
	return CoverageEntry{Group: g, Resource: r, Kind: k, Mode: WatchFull}
}

func hashedEntry(g, r, k string) CoverageEntry {
	return CoverageEntry{Group: g, Resource: r, Kind: k, Mode: WatchHashed}
}

// statusEntry is watched for status: a built-in kind with a status
// subresource, a discovery anchor or a preferred custom group.
func statusEntry(g, r, k string, hasStatus bool) CoverageEntry {
	return CoverageEntry{Group: g, Resource: r, Kind: k, Mode: WatchStatus,
		HasStatus: hasStatus}
}

// metadataEntry is watched for metadata only.
func metadataEntry(g, r, k string) CoverageEntry {
	return CoverageEntry{Group: g, Resource: r, Kind: k,
		Mode: WatchMetadata}
}

func excludedEntry(g, r, k string) CoverageEntry {
	return CoverageEntry{Group: g, Resource: r, Kind: k,
		Mode: CoverageModeExcluded}
}

const (
	gwGroup   = "gateway.networking.k8s.io"
	snapGroup = "snapshot.storage.k8s.io"
)

// CoverageCatalog lists every built-in listable kind of Kubernetes
// v1.35 and v1.36, the Gateway API and the volume snapshot kinds.
func CoverageCatalog() []CoverageEntry {
	var all []CoverageEntry
	for _, part := range [][]CoverageEntry{
		coreCoverage(), workloadCoverage(), networkCoverage(),
		storageCoverage(), policyCoverage(), clusterCoverage(),
		resourceCoverage(), customCoverage(),
	} {
		all = append(all, part...)
	}
	return all
}

func coreCoverage() []CoverageEntry {
	return []CoverageEntry{
		typedEntry("", "pods", "Pod"),
		typedEntry("", "nodes", "Node"),
		typedEntry("", "namespaces", "Namespace"),
		typedEntry("", "services", "Service"),
		excludedEntry("", "endpoints", "Endpoints"),
		hashedEntry("", "configmaps", "ConfigMap"),
		hashedEntry("", "secrets", "Secret"),
		typedEntry("", "serviceaccounts", "ServiceAccount"),
		typedEntry("", "persistentvolumes", "PersistentVolume"),
		typedEntry("", "persistentvolumeclaims", "PersistentVolumeClaim"),
		typedEntry("", "events", "Event"),
		excludedEntry("events.k8s.io", "events", "Event"),
		typedEntry("", "limitranges", "LimitRange"),
		typedEntry("", "resourcequotas", "ResourceQuota"),
		statusEntry("", "replicationcontrollers",
			"ReplicationController", true),
		metadataEntry("", "podtemplates", "PodTemplate"),
	}
}

func workloadCoverage() []CoverageEntry {
	return []CoverageEntry{
		typedEntry("apps", "deployments", "Deployment"),
		typedEntry("apps", "replicasets", "ReplicaSet"),
		typedEntry("apps", "statefulsets", "StatefulSet"),
		typedEntry("apps", "daemonsets", "DaemonSet"),
		metadataEntry("apps", "controllerrevisions", "ControllerRevision"),
		typedEntry("batch", "jobs", "Job"),
		typedEntry("batch", "cronjobs", "CronJob"),
		typedEntry("autoscaling", "horizontalpodautoscalers",
			"HorizontalPodAutoscaler"),
		typedEntry("policy", "poddisruptionbudgets", "PodDisruptionBudget"),
		metadataEntry("scheduling.k8s.io", "workloads", "Workload"),
		statusEntry("scheduling.k8s.io", "podgroups", "PodGroup", true),
		metadataEntry("scheduling.k8s.io", "priorityclasses",
			"PriorityClass"),
		metadataEntry("node.k8s.io", "runtimeclasses", "RuntimeClass"),
	}
}

func networkCoverage() []CoverageEntry {
	return []CoverageEntry{
		typedEntry("networking.k8s.io", "ingresses", "Ingress"),
		metadataEntry("networking.k8s.io", "ingressclasses",
			"IngressClass"),
		typedEntry("networking.k8s.io", "networkpolicies", "NetworkPolicy"),
		statusEntry("networking.k8s.io", "servicecidrs", "ServiceCIDR",
			true),
		metadataEntry("networking.k8s.io", "ipaddresses", "IPAddress"),
		typedEntry("discovery.k8s.io", "endpointslices", "EndpointSlice"),
	}
}

func storageCoverage() []CoverageEntry {
	return []CoverageEntry{
		typedEntry("storage.k8s.io", "storageclasses", "StorageClass"),
		typedEntry("storage.k8s.io", "volumeattachments",
			"VolumeAttachment"),
		metadataEntry("storage.k8s.io", "csidrivers", "CSIDriver"),
		metadataEntry("storage.k8s.io", "csinodes", "CSINode"),
		metadataEntry("storage.k8s.io", "csistoragecapacities",
			"CSIStorageCapacity"),
		metadataEntry("storage.k8s.io", "volumeattributesclasses",
			"VolumeAttributesClass"),
		statusEntry("storagemigration.k8s.io", "storageversionmigrations",
			"StorageVersionMigration", true),
		statusEntry(snapGroup, "volumesnapshots", "VolumeSnapshot", false),
		statusEntry(snapGroup, "volumesnapshotcontents",
			"VolumeSnapshotContent", false),
		statusEntry(snapGroup, "volumesnapshotclasses",
			"VolumeSnapshotClass", false),
	}
}

func policyCoverage() []CoverageEntry {
	return []CoverageEntry{
		metadataEntry("rbac.authorization.k8s.io", "roles", "Role"),
		metadataEntry("rbac.authorization.k8s.io", "rolebindings",
			"RoleBinding"),
		metadataEntry("rbac.authorization.k8s.io", "clusterroles",
			"ClusterRole"),
		metadataEntry("rbac.authorization.k8s.io", "clusterrolebindings",
			"ClusterRoleBinding"),
		typedEntry(admission, "mutatingwebhookconfigurations",
			"MutatingWebhookConfiguration"),
		typedEntry(admission, "validatingwebhookconfigurations",
			"ValidatingWebhookConfiguration"),
		statusEntry(admission, "validatingadmissionpolicies",
			"ValidatingAdmissionPolicy", true),
		metadataEntry(admission, "validatingadmissionpolicybindings",
			"ValidatingAdmissionPolicyBinding"),
		statusEntry(admission, "mutatingadmissionpolicies",
			"MutatingAdmissionPolicy", true),
		metadataEntry(admission, "mutatingadmissionpolicybindings",
			"MutatingAdmissionPolicyBinding"),
	}
}

func clusterCoverage() []CoverageEntry {
	return []CoverageEntry{
		statusEntry("apiextensions.k8s.io", "customresourcedefinitions",
			"CustomResourceDefinition", true),
		statusEntry("apiregistration.k8s.io", "apiservices", "APIService",
			true),
		statusEntry("certificates.k8s.io", "certificatesigningrequests",
			"CertificateSigningRequest", true),
		metadataEntry("certificates.k8s.io", "clustertrustbundles",
			"ClusterTrustBundle"),
		metadataEntry("coordination.k8s.io", "leases", "Lease"),
		metadataEntry("coordination.k8s.io", "leasecandidates",
			"LeaseCandidate"),
		statusEntry("flowcontrol.apiserver.k8s.io", "flowschemas",
			"FlowSchema", true),
		statusEntry("flowcontrol.apiserver.k8s.io",
			"prioritylevelconfigurations", "PriorityLevelConfiguration",
			true),
	}
}

func resourceCoverage() []CoverageEntry {
	return []CoverageEntry{
		statusEntry("resource.k8s.io", "resourceclaims", "ResourceClaim",
			true),
		metadataEntry("resource.k8s.io", "resourceclaimtemplates",
			"ResourceClaimTemplate"),
		metadataEntry("resource.k8s.io", "resourceslices",
			"ResourceSlice"),
		metadataEntry("resource.k8s.io", "deviceclasses", "DeviceClass"),
		statusEntry("resource.k8s.io", "devicetaintrules",
			"DeviceTaintRule", true),
	}
}

func customCoverage() []CoverageEntry {
	var out []CoverageEntry
	for _, r := range [][2]string{
		{"gatewayclasses", "GatewayClass"}, {"gateways", "Gateway"},
		{"httproutes", "HTTPRoute"}, {"grpcroutes", "GRPCRoute"},
		{"tlsroutes", "TLSRoute"}, {"tcproutes", "TCPRoute"},
		{"udproutes", "UDPRoute"}, {"referencegrants", "ReferenceGrant"},
		{"backendtlspolicies", "BackendTLSPolicy"},
		{"listenersets", "ListenerSet"},
	} {
		out = append(out, statusEntry(gwGroup, r[0], r[1], false))
	}
	// kwatch's own configuration is watched by the config CRD watcher.
	return append(out, excludedEntry("kwatch.abahmed.dev",
		"kwatchconfigs", "KwatchConfig"))
}
