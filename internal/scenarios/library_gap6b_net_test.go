package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// gap6bNetScenarios are volume attachment and mount plumbing, Services
// that route to nothing, and pod networking on a node.
func gap6bNetScenarios() []scenario {
	return []scenario{
		gap6bAttacherDown(), gap6bCSIDriverMissing(), gap6bFilesystemMismatch(),
		gap6bVolumeUnhealthy(), gap6bSelectorlessNoEndpoints(),
		gap6bEndpointSliceQuota(), gap6bDualStackMismatch(),
		gap6bCNIDeleteFails(), gap6bCNIConfigInvalid(), gap6bCNIPluginMissing(),
	}
}

// gap6bContainerCreating plays a replacement pod of w, on node, waiting in
// ContainerCreating with event reason and message every minute.
func gap6bContainerCreating(
	c *cluster, w *workload, idx int, node, reason, message string,
	rounds int32,
) {
	created := c.now
	for round := int32(1); round <= rounds; round++ {
		pod := w.pod(idx, node, createdAt(created),
			gapPending(waiting("ContainerCreating", "")))
		c.update(pod)
		gapKubelet(c, pod, reason, message, round)
		c.after(time.Minute)
	}
}

// gap6bAttacherDown: the attacher container of the CSI controller
// crash-loops; pods that need a volume attached wait in two namespaces.
func gap6bAttacherDown() scenario {
	return scenario{
		expect: g2Exp("csi-attacher-down-pods-wait-for-attach",
			"The attacher of a CSI driver crash-loops; new pods that "+
				"need a volume attached wait in ContainerCreating in two "+
				"namespaces with \"timed out waiting for "+
				"external-attacher\".",
			"deployment/kube-system/ebs-csi-controller", "notify",
			"node//n1", "node//n2", "deployment/shop/db",
			"deployment/billing/ledger").budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			ctl := csiController(c)
			c.list(ctl.objects())
			c.list(ctl.pod(0, "n1"), ctl.pod(1, "n2"))
			fleet := gap2Spread(c, []string{"shop/db", "billing/ledger"},
				[]string{"n1"})
			c.after(10 * time.Minute)
			for _, w := range fleet {
				*w.deployment.Spec.Replicas = 2
				w.setReady(1)
				c.update(w.objects())
			}
			for round := int32(1); round <= 6; round++ {
				c.update(ctl.pod(0, "n1", crashLoop(1, "Error", "csi-"+
					"attacher: failed to run: leader election lost",
					round)), ctl.pod(1, "n2", crashLoop(1, "Error", "csi-"+
					"attacher: failed to run: leader election lost", round)))
				ctl.setReady(0)
				c.update(ctl.objects())
				for _, w := range fleet {
					pod := w.pod(1, "n2", startedNow,
						gapPending(waiting("ContainerCreating", "")))
					c.update(pod)
					gapKubelet(c, pod, "FailedAttachVolume", "AttachVolume."+
						"Attach failed for volume \"pvc-7c1\" : timed out "+
						"waiting for external-attacher of ebs.csi.example."+
						"com CSI driver to attach volume vol-0a1b", round)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap6bCSIDriverMissing: a chart uninstall deleted the CSIDriver object of
// a hand-installed driver; the kubelet waits for an attachment nobody
// creates.
func gap6bCSIDriverMissing() scenario {
	return scenario{
		expect: g2Exp("csi-driver-object-missing-pod-waits",
			"The CSIDriver object of a driver was deleted; a pod that "+
				"uses its volume waits in ContainerCreating for a "+
				"volume attachment nobody creates.",
			"csidriver//hostpath.csi.example.com", "notify", "node//n1",
			"deployment/shop/db").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			w := c.deployment("shop", "db", "registry.example.com/db:5", 1)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.after(10 * time.Minute)
			c.remove(w.pod(0, "n1"))
			w.setReady(0)
			c.update(w.objects())
			gap6bContainerCreating(c, w, 1, "n1", "FailedMount",
				"MountVolume.WaitForAttach failed for volume \"pvc-7c1\" "+
					": kubernetes.io/csi: waiting for a volume attachment "+
					"for driver hostpath.csi.example.com: volume "+
					"attachment not found", 6)
		},
	}
}

// gap6bFilesystemMismatch: a volume restored from a snapshot holds a
// different filesystem than the node mounts.
func gap6bFilesystemMismatch() scenario {
	return scenario{
		expect: g2Exp("pv-filesystem-mismatch-pod-waits",
			"A restored volume holds a different filesystem than the "+
				"class asks for; the kubelet warns "+
				"FailedMountOnFilesystemMismatch on the volume and the "+
				"pod waits in ContainerCreating.",
			"persistentvolume//pv-restored", "notify", "node//n1",
			"deployment/shop/db").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			claim := storageBoundClaim(c, "shop", "data", "fast", "20Gi",
				"pv-restored")
			pv := storageVolume(c, claim, "pv-restored")
			c.list(scheduleClass(c, "fast", "Immediate"), pv, claim)
			w := c.deployment("shop", "db", "registry.example.com/db:5", 1)
			storageMount(w, claim.Name)
			w.setReady(0)
			c.create(w.objects())
			created := c.now
			for round := int32(1); round <= 6; round++ {
				pod := w.pod(0, "n1", createdAt(created),
					gapPending(waiting("ContainerCreating", "")))
				c.update(pod)
				g2Event(c, pv, "PersistentVolume",
					"FailedMountOnFilesystemMismatch", "Volume pv-restored "+
						"contains an ext4 filesystem; the class asks for "+
						"xfs: wrong fs type", "kubelet", round)
				c.after(time.Minute)
			}
		},
	}
}

// gap6bVolumeUnhealthy: the driver reports the claim's volume as not
// accessible; the database that uses it fails on I/O.
func gap6bVolumeUnhealthy() scenario {
	return scenario{
		expect: g2Exp("pvc-volume-health-inaccessible",
			"The driver reports a claim's volume as not accessible; "+
				"the database using it crashes on input/output errors "+
				"while its node is healthy.",
			"persistentvolumeclaim/inventory/pgdata", "notify",
			"node//n1", "deployment/inventory/postgres").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			claim := storageBoundClaim(c, "inventory", "pgdata", "fast",
				"20Gi", "pv-pgdata")
			c.list(scheduleClass(c, "fast", "Immediate"),
				storageVolume(c, claim, "pv-pgdata"), claim)
			w := c.deployment("inventory", "postgres",
				"registry.example.com/postgres:16.4", 1)
			storageMount(w, claim.Name)
			c.list(w.objects())
			c.list(w.pod(0, "n1"))
			c.after(5 * time.Minute)
			for m := int32(1); m <= 5; m++ {
				g2Event(c, claim, "PersistentVolumeClaim",
					"VolumeConditionAbnormal", "Volume pv-pgdata is not "+
						"accessible: the backend reports Inaccessible",
					"external-health-monitor", m)
			}
			storageCrashLoop(c, w, "n1", "PANIC: could not read block 12 "+
				"in file \"base/16384/1259\": Input/output error", 5)
		},
	}
}

// gap6bSelectorlessNoEndpoints: a Service without a selector for an
// external database has no Endpoints: its Endpoints object has another
// name.
func gap6bSelectorlessNoEndpoints() scenario {
	return scenario{
		expect: g2Exp("selectorless-service-without-endpoints",
			"A Service without a selector has no endpoints because its "+
				"Endpoints object carries another name; callers are "+
				"refused.", "service/shop/external-db", "notify",
			"deployment/shop/web", "node//n1").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			svc := clusterService(c, "shop", "external-db", 5432)
			svc.Spec.Selector = nil
			svc.Spec.ClusterIP = "10.96.40.9"
			c.list(svc)
			web := c.deployment("shop", "web", "registry.example.com/web:3", 2)
			c.list(web.objects())
			c.list(web.pod(0, "n1"), web.pod(1, "n2"))
			c.after(2 * time.Minute)
			message := "dial tcp 10.96.40.9:5432: connect: connection refused"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(web.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)), web.pod(1, "n2", crashLoop(1, "Error",
					message, restarts)))
				web.setReady(0)
				c.update(web.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gap6bEndpointSliceQuota: the namespace's object-count quota for
// EndpointSlices is full; scale-ups never get traffic and dead pods stay
// listed.
func gap6bEndpointSliceQuota() scenario {
	return scenario{
		expect: g2Exp("endpointslice-writes-refused-by-quota",
			"An object-count quota for EndpointSlices is full; the "+
				"controller warns FailedToUpdateEndpointSlices and the "+
				"Service keeps listing a pod that is gone.",
			"resourcequota/shop/object-counts", "notify",
			"service/shop/web", "node//n1").budget(3).
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			slices := corev1.ResourceName(
				"count/endpointslices.discovery.k8s.io")
			count := corev1.ResourceList{slices: resource.MustParse("20")}
			c.list(gap3Quota(c, "shop", "object-counts", count, count))
			w := c.deployment("shop", "web", "registry.example.com/web:7", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			svc := clusterService(c, "shop", "web", 8080)
			c.list(svc, clusterSlice(c, "shop", "web", w.pod(0, "n1"),
				w.pod(1, "n2")))
			c.after(10 * time.Minute)
			c.remove(w.pod(1, "n2"))
			c.create(w.pod(2, "n1", startedNow))
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				g2Event(c, svc, "Service", "FailedToUpdateEndpointSlices",
					"Error updating Endpoint Slices for Service shop/web: "+
						"failed to create EndpointSlice for Service "+
						"shop/web: endpointslices.discovery.k8s.io "+
						"\"web-x2\" is forbidden: exceeded quota: "+
						"object-counts", "endpoint-slice-controller", m)
			}
		},
	}
}

// gap6bDualStackMismatch: a Service is recreated with IPv6 as its primary
// family while the pods only get IPv4 addresses.
func gap6bDualStackMismatch() scenario {
	return scenario{
		expect: g2Exp("service-ipv6-family-pods-ipv4-only",
			"A Service whose primary IP family is IPv6 has no "+
				"endpoints because the cluster's pods only get IPv4 "+
				"addresses; its callers fail.",
			"service/shop/api", "notify", "deployment/shop/api",
			"node//n1").tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			api := c.deployment("shop", "api", "registry.example.com/api:4", 2)
			c.list(api.objects())
			first, second := api.pod(0, "n1"), api.pod(1, "n2")
			first.Status.PodIP, second.Status.PodIP = "10.244.1.7", "10.244.2.7"
			c.list(first, second)
			svc := clusterService(c, "shop", "api", 8080)
			svc.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv6Protocol}
			svc.Spec.ClusterIP = "fd00:10:96::2a"
			svc.Spec.ClusterIPs = []string{"fd00:10:96::2a"}
			slice := clusterSlice(c, "shop", "api")
			slice.AddressType = discoveryv1.AddressTypeIPv6
			c.list(svc, slice)
			web := c.deployment("shop", "web", "registry.example.com/web:3", 2)
			c.list(web.objects())
			c.list(web.pod(0, "n1"), web.pod(1, "n2"))
			c.after(2 * time.Minute)
			message := "dial tcp [fd00:10:96::2a]:8080: connect: " +
				"connection refused"
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(web.pod(0, "n1", crashLoop(1, "Error", message,
					restarts)), web.pod(1, "n2", crashLoop(1, "Error",
					message, restarts)))
				web.setReady(0)
				c.update(web.objects())
				c.after(time.Minute)
			}
		},
	}
}

// gap6bCNIDeleteFails: the CNI plugin of n2 cannot tear down sandboxes
// because its state is gone; deleted pods stay Terminating.
func gap6bCNIDeleteFails() scenario {
	return scenario{
		expect: g2Exp("node-cni-teardown-fails-pods-terminating",
			"After a CNI upgrade deleted pods on one node stay "+
				"Terminating with FailedKillPod \"failed to destroy "+
				"network for sandbox\"; rollouts stall.",
			"node//n2", "notify", "deployment/shop/orders",
			"deployment/shop/payments", "deployment/shop/cart").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			fleet := nodeFleet(c, "shop",
				[]string{"orders", "payments", "cart"}, []string{"n1", "n2"})
			c.after(10 * time.Minute)
			message := "failed to \"KillPodSandbox\" for \"6a1c\" with " +
				"KillPodSandboxError: \"rpc error: code = Unknown desc " +
				"= failed to destroy network for sandbox: plugin " +
				"type=\\\"calico\\\" failed (delete): error getting " +
				"ClusterInformation: connection is unauthorized\""
			for round := int32(1); round <= 5; round++ {
				for _, w := range fleet {
					pod := w.pod(1, "n2", notReady)
					deleting(&pod.ObjectMeta, c.now.Add(-time.Minute))
					c.update(pod)
					gapKubelet(c, pod, "FailedKillPod", message, round)
				}
				c.after(time.Minute)
			}
		},
	}
}

// gap6bCNIConfigInvalid: a CNI add-on upgrade left an invalid network
// configuration on n3; new pods there fail sandbox creation while the
// running ones are fine.
func gap6bCNIConfigInvalid() scenario {
	return scenario{
		expect: g2Exp("node-cni-config-invalid-new-pods-fail",
			"New pods on one node fail sandbox creation because the "+
				"node's CNI configuration is invalid (code 7); pods "+
				"on other nodes start.",
			"node//n3", "notify", "deployment/shop/api").
			tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"),
				c.node("n3", "zone-a"))
			w := c.deployment("shop", "api", "registry.example.com/api:3", 2)
			c.list(w.objects())
			c.list(w.pod(0, "n1"), w.pod(1, "n2"))
			c.after(10 * time.Minute)
			*w.deployment.Spec.Replicas = 3
			w.setReady(2)
			c.update(w.objects())
			gap6bContainerCreating(c, w, 2, "n3", "FailedCreatePodSandBox",
				"Failed to create pod sandbox: rpc error: code = Unknown "+
					"desc = failed to setup network for sandbox: plugin "+
					"type=\"bridge\" failed (add): invalid network config "+
					"(code 7): invalid CNI configuration: ipam: missing "+
					"field \"subnet\"", 6)
		},
	}
}

// gap6bCNIPluginMissing: the new node image lacks the portmap plugin; every
// new pod on n4, from three namespaces, fails sandbox creation.
func gap6bCNIPluginMissing() scenario {
	return scenario{
		expect: g2Exp("node-image-lacks-cni-plugin-binary",
			"Pods of three namespaces placed on a node with a new image "+
				"fail sandbox creation: the portmap plugin binary is "+
				"missing; other nodes are fine.",
			"node//n4", "notify", "deployment/shop/web",
			"deployment/billing/invoices", "deployment/media/transcode").
			budget(3).tail(10 * time.Minute),
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n4", "zone-a"))
			fleet := gap2Spread(c, []string{"shop/web", "billing/invoices",
				"media/transcode"}, []string{"n1"})
			c.after(10 * time.Minute)
			for _, w := range fleet {
				*w.deployment.Spec.Replicas = 2
				w.setReady(1)
				c.update(w.objects())
			}
			for round := int32(1); round <= 5; round++ {
				for _, w := range fleet {
					pod := w.pod(1, "n4", startedNow,
						gapPending(waiting("ContainerCreating", "")))
					c.update(pod)
					gapKubelet(c, pod, "FailedCreatePodSandBox", "Failed "+
						"to create pod sandbox: rpc error: code = Unknown "+
						"desc = failed to setup network for sandbox: "+
						"plugin type=\"portmap\" failed (add): failed to "+
						"find plugin \"portmap\" in path [/opt/cni/bin]",
						round)
				}
				c.after(time.Minute)
			}
		},
	}
}
