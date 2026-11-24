package scenarios

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// youngStuckPeerJoinsOldGroup: a controller was removed six weeks ago
// and two of its objects have waited on its finalizer since: one Low
// incident. An hour later one more of its objects is deleted and waits
// too. It joins the old group, which stays Low: nothing is sent at
// Needs attention, neither a message nor a material change.
func youngStuckPeerJoinsOldGroup() scenario {
	return scenario{
		expect: expectation{
			Name: "young-stuck-peer-joins-old-group",
			Description: "Objects of a controller removed weeks ago " +
				"wait on its finalizer; a new one deleted today joins " +
				"them: the group stays Low, nothing is sent.",
			Root: "finalizer/batch/jobs.example.com/cleanup",
			Tier: "digest", MaxMessages: 1, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			const finalizer = "jobs.example.com/cleanup"
			const old = 45 * 24 * time.Hour
			c.list(c.node("n1", "zone-a"))
			for _, name := range []string{"run-a", "run-b"} {
				c.list(stuckObjectAged(c, "batch", "v1", "ConfigMap",
					name, finalizer, old))
			}
			c.after(time.Hour)
			c.create(stuckObjectAged(c, "batch", "v1", "ConfigMap",
				"run-c", finalizer, 3*time.Hour))
			c.after(time.Hour)
		},
	}
}

// stuckObject is an object of namespace deleted leftAge ago and held by
// finalizer.
func stuckObject(
	c *cluster, namespace, apiVersion, kind, name, finalizer string,
) *unstructured.Unstructured {
	return stuckObjectAged(c, namespace, apiVersion, kind, name,
		finalizer, leftAge)
}

// stuckObjectAged is an object of namespace deleted age ago and held by
// finalizer.
func stuckObjectAged(
	c *cluster, namespace, apiVersion, kind, name, finalizer string,
	age time.Duration,
) *unstructured.Unstructured {
	meta := clusterMeta(c, namespace, name, kind)
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion, "kind": kind}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	stamp := metav1.NewTime(c.now.Add(-age))
	u.SetDeletionTimestamp(&stamp)
	u.SetFinalizers([]string{finalizer})
	return u
}
