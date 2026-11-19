package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// gap2ReleaseBScenarios are releases whose pod spec conflicts with the
// image, the identity or the node it runs on.
func gap2ReleaseBScenarios() []scenario {
	return []scenario{
		gap2SidecarPort(), gap2LowPortNonRoot(), gap2SubPathMount(),
		gap2MemoryEmptyDir(), gap2TokenAutomount(), gap2TokenSecret(),
		gap2SysctlForbidden(), gap2HostNetworkDNS(), gap2RegistryX509(),
	}
}

// gap2SidecarPort: a release adds a proxy sidecar that listens on 8080,
// the port the application binds too; the application cannot bind.
func gap2SidecarPort() scenario {
	return g2Release(g2Exp("release-sidecar-port-conflict",
		"A release adds a sidecar that binds the port the application "+
			"binds; the application exits with \"address already in "+
			"use\" at every start.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Containers = append(spec.Containers, corev1.Container{
				Name: "proxy", Image: "registry.example.com/proxy:1.4",
				Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
			})
		},
		g2Crash(1, "Error", "listen tcp :8080: bind: address already in "+
			"use"))
}

// gap2LowPortNonRoot: a hardening release sets runAsNonRoot; the
// application still binds port 80.
func gap2LowPortNonRoot() scenario {
	return g2Release(g2Exp("release-bind-low-port-non-root",
		"A hardening release runs the container as non-root while the "+
			"application binds port 80; it exits with \"permission "+
			"denied\" on bind at every start.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(_ *cluster, spec *corev1.PodSpec) {
			user := int64(10001)
			spec.Containers[0].SecurityContext = &corev1.SecurityContext{
				RunAsNonRoot: boolPtr(true), RunAsUser: &user,
			}
		},
		g2Crash(1, "Error", "listen tcp :80: bind: permission denied"))
}

// gap2SubPathMount: a release mounts one ConfigMap key with subPath over
// /etc/app, which the image has as a directory; the runtime refuses.
func gap2SubPathMount() scenario {
	return g2Release(g2Exp("release-subpath-not-a-directory",
		"A release mounts a ConfigMap key with subPath onto a "+
			"directory of the image; the container fails to be created "+
			"with \"not a directory\".",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(c *cluster, spec *corev1.PodSpec) {
			spec.Volumes = []corev1.Volume{configVolume("cfg",
				c.n("api-settings"))}
			spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{
				Name: "cfg", MountPath: "/etc/app", SubPath: "app.yaml"}}
		},
		func(c *cluster, w *workload) {
			c.list(configMap(c, "shop", "api-settings",
				map[string]string{"app.yaml": "port: 8080\n"}))
			g2Wait("RunContainerError", "Failed", "failed to create "+
				"containerd task: failed to create shim task: OCI "+
				"runtime create failed: runc create failed: unable to "+
				"start container process: error during container init: "+
				"error mounting \"/var/lib/kubelet/pods/6a1c/"+
				"volume-subpaths/cfg/app/0\" to rootfs at \"/etc/app\": "+
				"mount /var/lib/kubelet/pods/6a1c/volume-subpaths/cfg/"+
				"app/0:/etc/app (via /proc/self/fd/6), flags: 0x5001: "+
				"not a directory: unknown")(c, w)
		})
}

// gap2MemoryEmptyDir: a release moves the cache to a memory-backed
// emptyDir; what the cache holds counts against the 512Mi limit and the
// kernel kills the container at a heap that is small.
func gap2MemoryEmptyDir() scenario {
	return g2Release(g2Exp("release-memory-emptydir-oom",
		"A release stores its cache in a memory-backed emptyDir; the "+
			"files count against the memory limit and the container "+
			"is OOMKilled repeatedly.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Volumes = []corev1.Volume{{Name: "cache",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{
						Medium: corev1.StorageMediumMemory}}}}
			spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{
				Name: "cache", MountPath: "/var/cache/api"}}
			spec.Containers[0].Resources.Limits = corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("512Mi")}
		},
		g2Crash(137, "OOMKilled", ""))
}

// gap2TokenAutomount: a release sets automountServiceAccountToken false
// for an application that reads its in-cluster configuration.
func gap2TokenAutomount() scenario {
	return g2Release(g2Exp("release-token-automount-disabled",
		"A release disables automountServiceAccountToken for an "+
			"application that uses the in-cluster configuration; it "+
			"exits because the token file does not exist.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.AutomountServiceAccountToken = boolPtr(false)
		},
		g2Crash(1, "Error", "panic: unable to load in-cluster "+
			"configuration: open /var/run/secrets/kubernetes.io/"+
			"serviceaccount/token: no such file or directory"))
}

// gap2TokenSecret: a release mounts the secret api-token as a
// ServiceAccount token, which clusters since 1.24 no longer create.
func gap2TokenSecret() scenario {
	return g2Release(g2Exp("release-legacy-token-secret-missing",
		"A release mounts a service-account token Secret that was "+
			"never created, as clusters since 1.24 no longer generate "+
			"them; the new pod waits in ContainerCreating with "+
			"FailedMount \"secret not found\".",
		"secret/shop/api-token", "notify", "node//n1", "node//n2",
		"deployment/shop/api"),
		func(c *cluster, spec *corev1.PodSpec) {
			spec.Volumes = []corev1.Volume{{Name: "token",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: c.n("api-token")}}}}
			spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{
				Name: "token", MountPath: "/var/run/secrets/token"}}
		},
		func(c *cluster, w *workload) {
			g2Mount("MountVolume.SetUp failed for volume \"token\" : "+
				"secret \""+c.n("api-token")+"\" not found")(c, w)
		})
}

// gap2SysctlForbidden: a release asks for the unsafe sysctl
// kernel.msgmax; the kubelet rejects every pod the ReplicaSet creates.
func gap2SysctlForbidden() scenario {
	return g2Release(g2Exp("release-sysctl-forbidden",
		"A release asks for a sysctl the kubelet does not allow; every "+
			"new pod is rejected with SysctlForbidden and the old pods "+
			"keep serving.",
		"deployment/shop/api", "notify", "node//n1", "node//n2").
		tail(5*time.Minute),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.SecurityContext = &corev1.PodSecurityContext{
				Sysctls: []corev1.Sysctl{{Name: "kernel.msgmax",
					Value: "65536"}}}
		},
		func(c *cluster, w *workload) {
			message := "forbidden sysctl: \"kernel.msgmax\" not allowlisted"
			for i := range 5 {
				pod := w.pod(i, "n1", g2Failed("SysctlForbidden", message))
				c.create(pod)
				gapKubelet(c, pod, "SysctlForbidden", message, 1)
				c.after(time.Minute)
			}
		})
}

// gap2HostNetworkDNS: a release moves the pods to the host network and
// keeps dnsPolicy ClusterFirst, which the kubelet then ignores; service
// names stop resolving.
func gap2HostNetworkDNS() scenario {
	return g2Release(g2Exp("release-host-network-cluster-dns",
		"A release turns on hostNetwork with the default DNS policy; "+
			"the pods resolve with the node's resolver and fail to "+
			"look up cluster service names.",
		"deployment/shop/api", "notify", "node//n1", "node//n2",
		"cluster-dns//cluster-dns"),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.HostNetwork = true
			spec.DNSPolicy = corev1.DNSClusterFirst
		},
		g2Crash(1, "Error", "dial tcp: lookup ledger.shop.svc.cluster."+
			"local on 10.0.0.2:53: no such host"))
}

// gap2RegistryX509: a release pulls its image from a registry whose
// certificate comes from an internal CA the nodes do not trust.
func gap2RegistryX509() scenario {
	image := "registry.corp.example/shop/api:2.6"
	return g2Release(g2Exp("release-registry-unknown-authority",
		"A release pulls from a private registry whose certificate is "+
			"signed by an authority the nodes do not trust; the pull "+
			"fails with \"x509: certificate signed by unknown "+
			"authority\".",
		"registry//registry.corp.example", "notify", "node//n1",
		"node//n2", "deployment/shop/api"),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Containers[0].Image = image
		},
		g2Pull("Failed to pull image \""+image+"\": failed to pull and "+
			"unpack image \""+image+"\": failed to resolve reference \""+
			image+"\": failed to do request: Head \"https://registry."+
			"corp.example/v2/shop/api/manifests/2.6\": tls: failed to "+
			"verify certificate: x509: certificate signed by unknown "+
			"authority"))
}
