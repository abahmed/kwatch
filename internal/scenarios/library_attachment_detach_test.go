package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
)

// attachmentDetachScenarios are attachments whose driver runs and keeps
// failing to detach: its finalizer is handled, the detach cannot finish.
func attachmentDetachScenarios() []scenario {
	return []scenario{attachmentsDetachErrorVolumeGone()}
}

// attachmentsDetachErrorVolumeGone: two attachments of one storage
// driver were deleted a day ago. Their nodes and claims' volumes are
// still listed, but the backing disks were removed, so every detach
// returns "not found" and the attacher never removes its finalizer.
// Each attachment is its own root; the finalizer is not unhandled.
func attachmentsDetachErrorVolumeGone() scenario {
	return scenario{
		expect: expectation{
			Name: "attachments-detach-error-disk-gone",
			Description: "Two VolumeAttachments are stuck deleting; " +
				"their driver keeps failing to detach disks that no " +
				"longer exist.",
			Root:        "volumeattachment//csi-7a7a7a",
			OtherRoots:  []string{"volumeattachment//csi-8b8b8b"},
			Tier:        "notify",
			MaxMessages: 2,
			MustNotBlame: []string{
				"finalizer//external-attacher/disk-csi-example-com"},
			Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
			for i, name := range []string{"csi-7a7a7a", "csi-8b8b8b"} {
				volume := "pvc-" + name[4:]
				node := []string{"n1", "n2"}[i]
				c.list(&corev1.PersistentVolume{
					ObjectMeta: c.meta("", volume)})
				c.list(detachFailing(c, name, volume, node))
			}
		},
	}
}

// detachFailing is a VolumeAttachment deleted a day ago whose driver
// reports that the disk it should detach is not found.
func detachFailing(
	c *cluster, name, volume, node string,
) *storagev1.VolumeAttachment {
	va := staleAttachment(c, name, volume, node)
	va.Finalizers = []string{"external-attacher/disk-csi-example-com"}
	va.Spec.Attacher = "disk.csi.example.com"
	at := va.DeletionTimestamp.Time.Add(364 * 24 * time.Hour)
	va.DeletionTimestamp.Time = at
	va.Status.DetachError = &storagev1.VolumeError{
		Message: "rpc error: code = NotFound desc = disk " + volume +
			" not found"}
	return va
}
