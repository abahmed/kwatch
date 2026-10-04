package incident

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An announced incident keeps the loudest tier it reached. A crash loop
// whose critical member clears for a moment must not flip from page to
// notify and back, each flip a message.
func TestManagerAnnouncedTierNeverDrops(t *testing.T) {
	r := newRig(t, Config{})
	node := entity("node", "n1")
	critical := sig(node, "NodeNotReady", detection.Critical)
	warning := sig(node, "NodeResourceHigh", detection.Warning)
	r.raise(at(0), critical, warning)
	ds := r.tick(at(DefaultPageSettle))
	wantAction(t, ds, Announce, "settled")
	if ds[0].Incident.Tier != Page {
		t.Fatalf("tier = %v, want Page", ds[0].Incident.Tier)
	}

	r.clear(at(time.Minute), critical)
	wantNone(t, r.tick(at(time.Minute)))
	if got := r.of(node).Tier; got != Page {
		t.Fatalf("tier dropped to %v while the incident is open", got)
	}
}

// Before the first message the tier still follows the members: a page
// that calms down during its settle is announced as a notify.
func TestManagerSettlingTierFollowsMembers(t *testing.T) {
	r := newRig(t, Config{})
	pod := entity(kube.KindPod, "web")
	critical := sig(pod, "OOMKilled", detection.Critical)
	warning := sig(pod, "HighRestartCount", detection.Warning)
	r.raise(at(0), critical, warning)
	r.clear(at(5*time.Second), critical)

	ds := r.tick(at(DefaultSettle))

	wantAction(t, ds, Announce, "settled")
	if ds[0].Incident.Tier == Page {
		t.Fatal("a settling incident must follow its members' tier")
	}
}
