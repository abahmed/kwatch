package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// gap5bReleaseScenarios are releases and specs whose mounts, names and
// mount propagation conflict with what the image or the node provides.
func gap5bReleaseScenarios() []scenario {
	return []scenario{
		gap5bVolumeHidesFiles(), gap5bWriteToConfigMapVolume(),
		gap5bExternalNameIP(), gap5bImageTagsRemoved(),
		gap5bHostMountPropagation(), gap5bPostStartHangs(),
		gap5bOrdinalRefusesRestart(),
	}
}

// gap5bVolumeHidesFiles: a release mounts a volume over the directory
// where the image keeps its configuration; the mount is empty.
func gap5bVolumeHidesFiles() scenario {
	return g2Release(g2Exp("release-volume-hides-image-files",
		"A release mounts an empty volume over the directory where the "+
			"image keeps its configuration; the container exits at "+
			"once on a missing file under the mount.",
		"deployment/shop/api", "notify", "node//n1", "node//n2"),
		func(c *cluster, spec *corev1.PodSpec) {
			spec.Volumes = []corev1.Volume{{Name: "conf",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.
						PersistentVolumeClaimVolumeSource{
						ClaimName: c.n("pgconf")}}}}
			spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{
				Name: "conf", MountPath: "/etc/postgresql"}}
		},
		func(c *cluster, w *workload) {
			claim := storageBoundClaim(c, "shop", "pgconf", "fast", "1Gi",
				"pv-pgconf")
			c.list(scheduleClass(c, "fast", "Immediate"),
				storageVolume(c, claim, "pv-pgconf"), claim)
			g2Crash(1, "Error", "FATAL: could not open file \"/etc/"+
				"postgresql/postgresql.conf\": No such file or "+
				"directory")(c, w)
		})
}

// gap5bWriteToConfigMapVolume: the entrypoint rewrites a file inside a
// ConfigMap volume, which is read-only.
func gap5bWriteToConfigMapVolume() scenario {
	return g2Release(g2Exp("release-writes-into-configmap-volume",
		"A release mounts a ConfigMap at the directory the entrypoint "+
			"rewrites at start; the container exits with \"read-only "+
			"file system\".",
		"deployment/shop/api", "notify", "configmap/shop/app-config",
		"node//n1", "node//n2"),
		func(c *cluster, spec *corev1.PodSpec) {
			spec.Volumes = []corev1.Volume{configVolume("conf",
				c.n("app-config"))}
			spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{
				Name: "conf", MountPath: "/etc/app"}}
		},
		func(c *cluster, w *workload) {
			c.list(configMap(c, "shop", "app-config",
				map[string]string{"app.yaml": "port: 8080\n"}))
			g2Crash(1, "Error", "sed: couldn't open temporary file /etc/"+
				"app/sedXk2: Read-only file system")(c, w)
		})
}

// gap5bExternalNameIP: a Service of type ExternalName names an IP address;
// its DNS name does not resolve for the callers.
func gap5bExternalNameIP() scenario {
	return scenario{
		expect: g2Exp("service-external-name-is-an-ip-address",
			"A Service of type ExternalName whose target is an IP "+
				"address does not resolve; its callers fail with \"no "+
				"such host\".",
			"service/shop/db", "notify", "deployment/shop/web",
			"cluster-dns//cluster-dns", "node//n1").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			svc := clusterService(c, "shop", "db", 5432)
			svc.Spec.Type = corev1.ServiceTypeExternalName
			svc.Spec.ExternalName = "10.0.5.12"
			svc.Spec.Selector = nil
			c.list(svc)
			c.probe(kube.ClusterDNS, "")
			web := c.deployment("shop", "web", "registry.example.com/web:3", 2)
			c.list(web.objects())
			c.list(web.pod(0, "n1"), web.pod(1, "n2"))
			c.after(2 * time.Minute)
			message := "dial tcp: lookup db.shop.svc.cluster.local on " +
				"10.96.0.10:53: no such host"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(web.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)), web.pod(1, "n2", crashLoop(1, "Error",
					message, restarts)))
				web.setReady(0)
				c.update(web.objects())
				c.probe(kube.ClusterDNS, "")
				c.after(time.Minute)
			}
		},
	}
}

// gap5bImageTagsRemoved: a public registry stops serving the tags of a
// base image; the pods that run keep running, and every new pod of three
// namespaces fails to pull.
func gap5bImageTagsRemoved() scenario {
	return scenario{
		expect: g2Exp("registry-removes-tags-shared-by-workloads",
			"A registry stops serving the tags of an image used by "+
				"workloads in three namespaces; their running pods are "+
				"fine and every new pod fails with \"manifest unknown\".",
			"registry//registry.example.com", "notify",
			"deployment/shop/web", "deployment/billing/invoices",
			"deployment/media/transcode", "node//n1").budget(3).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := gap2Spread(c, []string{"shop/web", "billing/invoices",
				"media/transcode"}, []string{"n1"})
			c.after(10 * time.Minute)
			message := "Failed to pull image \"registry.example.com/" +
				"vendor/base:1.2\": rpc error: code = NotFound desc = " +
				"failed to pull and unpack image: manifest unknown"
			for _, w := range fleet {
				*w.deployment.Spec.Replicas = 2
				w.setReady(1)
				c.update(w.objects())
			}
			for round := int32(1); round <= 6; round++ {
				for _, w := range fleet {
					reason := "ErrImagePull"
					if round%2 == 0 {
						reason = "ImagePullBackOff"
					}
					pod := w.pod(1, "n2", startedNow,
						gapPending(waiting(reason, message)))
					c.update(pod)
					gapKubelet(c, pod, "Failed", message, round)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap5bHostMountPropagation: the node agent asks for rshared propagation
// on the host's root, which the runtime refuses on every node.
func gap5bHostMountPropagation() scenario {
	return scenario{
		expect: g2Exp("daemonset-host-root-mount-not-shared",
			"A node agent DaemonSet mounts the host root with shared "+
				"propagation; every pod fails with \"is mounted on / "+
				"but it is not a shared or slave mount\".",
			"daemonset/monitoring/host-agent", "notify", "node//n1",
			"node//n2", "node//n3").tail(10 * time.Minute),
		build: func(c *cluster) {
			nodes := []string{"n1", "n2", "n3"}
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			ds := g2Daemon(c, "monitoring", "host-agent", nodes...)
			c.after(5 * time.Minute)
			message := "failed to generate container spec: path / is " +
				"mounted on / but it is not a shared or slave mount"
			for round := int32(1); round <= 6; round++ {
				setAgentCounts(ds, 3, 0)
				c.update(ds)
				for _, node := range nodes {
					pod := agentPod(c, ds, node, gapPending(waiting(
						"CreateContainerError", message)))
					c.update(pod)
					gapKubelet(c, pod, "Failed", "Error: "+message, round)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap5bPostStartHangs: a release adds a postStart hook that never
// returns; the container runs but the pod never leaves ContainerCreating.
func gap5bPostStartHangs() scenario {
	return g2Release(g2Exp("release-post-start-hook-hangs",
		"A release adds a postStart hook that never returns; the new "+
			"pod stays in ContainerCreating with no event for ten "+
			"minutes.", "deployment/shop/api", "notify", "node//n1",
		"node//n2").tail(10*time.Minute),
		func(_ *cluster, spec *corev1.PodSpec) {
			spec.Containers[0].Lifecycle = &corev1.Lifecycle{
				PostStart: &corev1.LifecycleHandler{Exec: &corev1.ExecAction{
					Command: []string{"/bin/sh", "-c", "/app/wait-for-peer"}}}}
		},
		func(c *cluster, w *workload) {
			gapRounds(c, w, 10, time.Minute, func(int32) podState {
				return gapPending(waiting("ContainerCreating", ""))
			}, nil)
		})
}

// gap5bOrdinalRefusesRestart: after a node drain one broker of a clustered
// StatefulSet reads its old cluster state from its volume and refuses to
// start; the other ordinals keep running.
func gap5bOrdinalRefusesRestart() scenario {
	return scenario{
		expect: g2Exp("statefulset-ordinal-refuses-restart-after-drain",
			"After a node drain one ordinal of a clustered StatefulSet "+
				"exits at start on stale cluster state while the "+
				"others run and nothing was released.",
			"statefulset/streaming/queue", "notify", "node//n1",
			"node//n2").tail(10 * time.Minute),
		build: func(c *cluster) {
			sts := scaleQueue(c)
			c.after(10 * time.Minute)
			advCordon(c, c.node("n1", "zone-a"))
			c.remove(operatorBroker(c, sts, 2, "n1"))
			message := "refusing to start: previous cluster state found on " +
				"disk, not safe to bootstrap; rejoin required"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(operatorBroker(c, sts, 2, "n2", startedNow,
					crashLoop(1, "Error", message, restarts)))
				c.after(time.Minute)
			}
		},
	}
}
