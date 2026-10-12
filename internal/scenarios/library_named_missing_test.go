package scenarios

import (
	"time"

	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// namedMissingScenarios are failures whose cause the cluster states
// outright: the object they name does not exist, or the limit they hit
// is their own.
func namedMissingScenarios() []scenario {
	return []scenario{claimNamesMissingStorageClass(),
		apiServiceBackendMissing(), podEvictedOverEphemeralLimit()}
}

// claimNamesMissingStorageClass: a journal database's claim names a
// StorageClass nobody created, so no volume is ever provisioned and its
// pod cannot be scheduled. The missing class is the root.
func claimNamesMissingStorageClass() scenario {
	return scenario{
		expect: expectation{
			Name: "claim-names-missing-storage-class",
			Description: "A claim asks for a StorageClass that does not " +
				"exist; provisioning fails and its pod stays pending.",
			Root: "storageclass//nvme-archive", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//w2"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("w2", "zone-b"), c.node("w3", "zone-b"))
			c.list(scheduleClass(c, "standard",
				storagev1.VolumeBindingImmediate))
			claim := scheduleClaim(c, "ledger", "journal", "nvme-archive")
			c.create(claim)
			w := c.deployment("ledger", "journal-db",
				"registry.example.com/journal:9", 1)
			storageMount(w, claim.Name)
			w.setReady(0)
			c.create(w.objects())
			message := "0/2 nodes are available: pod has unbound " +
				"immediate PersistentVolumeClaims."
			c.create(w.pod(0, "", pendingUnscheduled(message)))
			provision := "storageclass.storage.k8s.io \"" +
				c.n("nvme-archive") + "\" not found"
			for m := int32(1); m <= 6; m++ {
				c.after(time.Minute)
				c.warn(c.warningEvent(claim, "PersistentVolumeClaim",
					"ProvisioningFailed", provision,
					"persistentvolume-controller", m))
			}
		},
	}
}

// apiServiceBackendMissing: an aggregated API is registered against a
// Service that was never created, so the API server reports it
// unavailable with "service not found". The missing Service is the root.
func apiServiceBackendMissing() scenario {
	return scenario{
		expect: expectation{
			Name: "apiservice-backend-service-missing",
			Description: "An APIService points at a Service that does " +
				"not exist and reports itself unavailable.",
			Root: "service/reports/report-adapter", Tier: "notify",
			MaxMessages: 2, MustNotBlame: []string{"node//w2"},
			Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("w2", "zone-b"))
			api := reportAPIService(c, "False", "ServiceNotFound",
				"service/report-adapter in \"reports\" is not present")
			c.list(api)
			c.after(10 * time.Minute)
			c.update(reportAPIService(c, "False", "ServiceNotFound",
				"service/report-adapter in \"reports\" is not present"))
		},
	}
}

func reportAPIService(
	c *cluster, status, reason, message string,
) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
		"metadata": map[string]any{
			"name": "v1alpha1.reports.acme.example", "uid": "apiservice-rep",
		},
		"spec": map[string]any{
			"group": "reports.acme.example", "version": "v1alpha1",
			"service": map[string]any{
				"namespace": c.n("reports"),
				"name":      c.n("report-adapter"),
			},
		},
		"status": map[string]any{"conditions": []any{map[string]any{
			"type": "Available", "status": status, "reason": reason,
			"message":            message,
			"lastTransitionTime": c.start.UTC().Format(time.RFC3339),
		}}},
	}}
}

// podEvictedOverEphemeralLimit: a renderer writes scratch files past
// the 64Mi ephemeral-storage limit of its container; the kubelet evicts
// every pod the Deployment starts. The limit is the workload's own.
func podEvictedOverEphemeralLimit() scenario {
	return scenario{
		expect: expectation{
			Name: "pod-evicted-over-ephemeral-limit",
			Description: "A pod is evicted again and again for using " +
				"more local ephemeral storage than its container limit.",
			Root: "deployment/render/thumbnailer", Tier: "notify",
			MaxMessages: 3, MustNotBlame: []string{"node//w2"},
			Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("w2", "zone-b"), c.node("w3", "zone-b"))
			w := c.deployment("render", "thumbnailer",
				"registry.example.com/thumbnailer:2.1", 1)
			c.list(w.objects())
			c.list(w.pod(0, "w2"))
			c.after(2 * time.Minute)
			message := "Pod ephemeral local storage usage exceeds the " +
				"total limit of containers 64Mi."
			for round := int32(1); round <= 3; round++ {
				pod := w.pod(int(round), "w2", evicted(message))
				c.update(pod)
				c.warn(c.warningEvent(pod, "Pod", "Evicted", message,
					"kubelet", 1))
				w.setReady(0)
				c.update(w.deployment, w.replicaSet)
				c.after(2 * time.Minute)
			}
		},
	}
}
