package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
)

// provisionerScaledToZero: the CSI controller was scaled to zero, so it
// has no pods at all. The claim waits on it: the controller is the root,
// not "no workload found".
func provisionerScaledToZero() scenario {
	return scenario{
		expect: expectation{
			Name: "provisioner-controller-scaled-to-zero",
			Description: "A claim stays Pending because the controller " +
				"Deployment that serves its provisioner has no pods.",
			Root: "deployment/kube-system/ebs-csi-controller",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"storageclass//gp"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			class := scheduleClass(c, "gp", storagev1.VolumeBindingImmediate)
			class.Provisioner = "ebs.csi.example.com"
			c.list(class)
			ctl := csiController(c)
			ctl.deployment.Spec.Replicas = new(int32)
			ctl.setReady(0)
			c.list(ctl.objects())
			c.after(time.Hour)
			claim := scheduleClaim(c, "data", "orders-db", "gp")
			c.create(claim)
			provisionFailures(c, claim, "ebs.csi.example.com", "gp")
		},
	}
}

// provisionerLookAlike: a scaled-to-zero Deployment shares only the
// first word of the provisioner's name ("ebs-billing-controller" for
// "ebs.csi.example.com"). It is not what serves the class, so the claim
// stays the root and the billing Deployment is not blamed.
func provisionerLookAlike() scenario {
	return scenario{
		expect: expectation{
			Name: "provisioner-look-alike-workload",
			Description: "A workload whose name only resembles the " +
				"provisioner's is not blamed for an unbound claim.",
			Root: "persistentvolumeclaim/data/orders-db",
			Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{
				"deployment/kube-system/ebs-billing-controller"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			class := scheduleClass(c, "gp", storagev1.VolumeBindingImmediate)
			class.Provisioner = "ebs.csi.example.com"
			c.list(class)
			billing := c.deployment("kube-system", "ebs-billing-controller",
				"registry.example.com/billing:2", 1)
			billing.deployment.Spec.Replicas = new(int32)
			billing.setReady(0)
			c.list(billing.objects())
			c.after(time.Hour)
			claim := scheduleClaim(c, "data", "orders-db", "gp")
			c.create(claim)
			provisionFailures(c, claim, "ebs.csi.example.com", "gp")
		},
	}
}

// provisionFailures is six minutes of ProvisioningFailed events on the
// claim.
func provisionFailures(
	c *cluster, claim *corev1.PersistentVolumeClaim, source, class string,
) {
	said := "failed to provision volume with StorageClass \"" + class +
		"\": rpc error: code = Internal desc = could not create the disk"
	for m := int32(1); m <= 6; m++ {
		c.after(time.Minute)
		c.warn(c.warningEvent(claim,
			"PersistentVolumeClaim", "ProvisioningFailed", said, source, m))
	}
}
