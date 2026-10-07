package scenarios

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// volumeFullScenarios are claims the kubelet reports as full, by bytes
// or by inodes, with the workloads that mount them.
func volumeFullScenarios() []scenario {
	return []scenario{
		volumeInodesFull(), volumeFullNamesUsers(), volumeNearlyFullQuiet(),
	}
}

// volumeStats is one kubelet reading of a claim: bytes used of capacity
// and the share of inodes in use.
func volumeStats(c *cluster, namespace, name string, used, capacity,
	inodesPct float64) inventory.Observation {
	return inventory.Observation{
		Kind: inventory.Observed, Source: kube.StatsSource, At: c.now,
		Entity: inventory.CoreID(kube.KindPVC, c.n(namespace), c.n(name)),
		Attributes: map[string]inventory.Value{
			kube.AttrVolumeUsedPct:       inventory.Number(100 * used / capacity),
			kube.AttrVolumeUsedBytes:     inventory.Number(used),
			kube.AttrVolumeCapacityBytes: inventory.Number(capacity),
			kube.AttrVolumeInodesPct:     inventory.Number(inodesPct),
		},
	}
}

const gib = float64(1 << 30)

// volumeInodesFull: a mail relay's 100Gi spool holds millions of tiny
// files. Its bytes are 12% used but every inode is taken, so each write
// fails with "No space left on device" and the relay crash-loops. The
// claim is the root, and it is out of inodes, not bytes.
func volumeInodesFull() scenario {
	return scenario{
		expect: expectation{
			Name: "volume-inodes-full",
			Description: "A mail relay's PersistentVolumeClaim runs out " +
				"of inodes with 88% of its bytes free; the relay " +
				"crash-loops with \"No space left on device\".",
			Root: "persistentvolumeclaim/mail/spool", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//w1", "storageclass//standard"},
		},
		build: func(c *cluster) {
			c.list(c.node("w1", "zone-a"))
			claim := storageBoundClaim(c, "mail", "spool", "standard",
				"100Gi", "pv-spool")
			c.list(scheduleClass(c, "standard", "Immediate"), claim)
			w := c.deployment("mail", "relay",
				"registry.example.com/relay:2.4", 1)
			storageMount(w, claim.Name)
			c.list(w.objects())
			c.list(w.pod(0, "w1"))
			c.after(3 * time.Minute)
			c.emit(volumeStats(c, "mail", "spool", 12*gib, 100*gib, 99))
			message := "queue: cannot create /var/spool/relay/new/" +
				"1760000000.M1P7: No space left on device"
			storageCrashLoop(c, w, "w1", message, 4)
		},
	}
}

// volumeFullNamesUsers: the database's 20Gi claim is 98% used and two
// workloads mount it. The database crash-loops on a full disk. The
// claim is the root, and the message says how full it is and who uses
// it.
func volumeFullNamesUsers() scenario {
	return scenario{
		expect: expectation{
			Name: "volume-full-names-users",
			Description: "A claim two workloads mount is 98% used; the " +
				"database crash-loops with \"No space left on device\".",
			Root: "persistentvolumeclaim/inventory/pgdata", Tier: "notify",
			MaxMessages:  2,
			MustNotBlame: []string{"node//w1", "storageclass//standard"},
		},
		build: func(c *cluster) {
			c.list(c.node("w1", "zone-a"))
			claim := storageBoundClaim(c, "inventory", "pgdata",
				"standard", "20Gi", "pv-pgdata")
			c.list(scheduleClass(c, "standard", "Immediate"), claim)
			db := c.deployment("inventory", "postgres",
				"registry.example.com/postgres:16.4", 1)
			storageMount(db, claim.Name)
			report := c.deployment("inventory", "reporter",
				"registry.example.com/reporter:1.9", 1)
			storageMount(report, claim.Name)
			report.setReady(1)
			c.list(db.objects())
			c.list(report.objects())
			c.list(db.pod(0, "w1"), report.pod(0, "w1"))
			c.after(5 * time.Minute)
			c.emit(volumeStats(c, "inventory", "pgdata", 19.6*gib,
				20*gib, 3))
			message := "PANIC:  could not write to file " +
				"\"pg_wal/xlogtemp.31\": No space left on device"
			storageCrashLoop(c, db, "w1", message, 5)
		},
	}
}

// volumeNearlyFullQuiet: a claim is 96% used but nothing fails yet. The
// finding is real and the claim is its subject; nothing else is blamed.
func volumeNearlyFullQuiet() scenario {
	return scenario{
		expect: expectation{
			Name: "volume-nearly-full-quiet",
			Description: "A claim is 96% used and its workload still " +
				"runs; the full claim alone is reported.",
			Root: "persistentvolumeclaim/files/uploads", Tier: "notify",
			MaxMessages:  1,
			MustNotBlame: []string{"node//w1"},
		},
		build: func(c *cluster) {
			c.list(c.node("w1", "zone-a"))
			claim := storageBoundClaim(c, "files", "uploads", "standard",
				"50Gi", "pv-uploads")
			c.list(scheduleClass(c, "standard", "Immediate"), claim)
			w := c.deployment("files", "gallery",
				"registry.example.com/gallery:7.1", 1)
			storageMount(w, claim.Name)
			w.setReady(1)
			c.list(w.objects())
			c.list(w.pod(0, "w1"))
			c.after(10 * time.Minute)
			c.emit(volumeStats(c, "files", "uploads", 48*gib, 50*gib, 20))
			c.after(10 * time.Minute)
			c.emit(volumeStats(c, "files", "uploads", 48.2*gib, 50*gib, 20))
		},
	}
}
