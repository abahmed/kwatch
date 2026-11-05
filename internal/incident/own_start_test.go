package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ownConfig is the watcher running as Deployment "watcher" in "watch".
var ownConfig = Config{OwnNamespace: "watch", OwnWorkload: "watcher"}

// ownWait is the watcher's own pod in namespace "watch" waiting for its
// volume, raised at d after the manager first ran.
func ownWait(t *testing.T, d time.Duration) (*rig, detection.Finding) {
	r := newRig(t, ownConfig)
	r.tick(at(0))
	pod := ownedPod(r, "watcher", "watcher-7d4f9c6b8d-x2k9p")
	wait := sig(pod, reasons.VolumeAttachWaiting, detection.Warning)
	r.raise(at(d), wait)
	return r, wait
}

// Right after kwatch starts, its own pod waiting for its volume is the
// restart itself: no message.
func TestOwnVolumeWaitAtStartIsSilent(t *testing.T) {
	r, _ := ownWait(t, time.Minute)
	wantNone(t, r.tick(at(5*time.Minute)))
	if got := r.only(); got.Tier != Silent {
		t.Fatalf("tier = %v, want silent", got.Tier)
	}
}

// A wait that outlasts the window is news like any other.
func TestOwnVolumeWaitPastTheWindowAlerts(t *testing.T) {
	r, _ := ownWait(t, time.Minute)
	r.tick(at(5 * time.Minute))
	r.tick(at(OwnStartWindow + time.Minute))
	if got := r.only(); got.Tier == Silent {
		t.Fatalf("tier = %v, want it told after the window", got.Tier)
	}
}

// Someone else's pod waiting for its volume is not kwatch's restart.
func TestOtherVolumeWaitAtStartAlerts(t *testing.T) {
	r := newRig(t, ownConfig)
	r.tick(at(0))
	pod := inventory.CoreID(kube.KindPod, "shop", "db-0")
	r.raise(at(time.Minute),
		sig(pod, reasons.VolumeAttachWaiting, detection.Warning))
	if got := r.only(); got.Tier == Silent {
		t.Fatalf("tier = %v, want it told", got.Tier)
	}
}

// ownedPod is a pod of Deployment deploy in "watch", through a
// ReplicaSet, as Kubernetes records the owners.
func ownedPod(r *rig, deploy, name string) inventory.EntityID {
	d := inventory.NewEntityID("apps", kube.KindDeployment, "watch", deploy)
	rs := inventory.NewEntityID("apps", kube.KindReplicaSet, "watch", deploy+"-rs")
	pod := inventory.CoreID(kube.KindPod, "watch", name)
	r.relate(pod, inventory.OwnedBy, rs)
	r.relate(rs, inventory.OwnedBy, d)
	return pod
}

// Another workload in kwatch's own namespace waiting for its volume is
// not kwatch's restart either, nor is any pod when kwatch does not know
// its own workload.
func TestSameNamespaceOtherWorkloadAtStartAlerts(t *testing.T) {
	for name, cfg := range map[string]Config{
		"other workload":  ownConfig,
		"own one unknown": {OwnNamespace: "watch"},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, cfg)
			r.tick(at(0))
			pod := ownedPod(r, "watcher-db", "watcher-db-6c9d-q8w2z")
			if name == "own one unknown" {
				pod = ownedPod(r, "watcher", "watcher-7d4f9c6b8d-x2k9p")
			}
			r.raise(at(time.Minute),
				sig(pod, reasons.VolumeAttachWaiting, detection.Warning))
			if got := r.only(); got.Tier == Silent {
				t.Fatalf("tier = %v, want it told", got.Tier)
			}
		})
	}
}

func TestOwnWorkloadOf(t *testing.T) {
	for pod, want := range map[string]string{
		"kwatch-5569b48575-bmfbl":   "kwatch",
		"my-watch-7d4f9c6b8d-x2k9p": "my-watch",
		"watcher-0":                 "",
		"":                          "",
		"kwatch-Upper-case":         "",
	} {
		if got := OwnWorkloadOf(pod); got != want {
			t.Errorf("OwnWorkloadOf(%q) = %q, want %q", pod, got, want)
		}
	}
}
