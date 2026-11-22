package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// nfsConsumer is a replica that mounts a claim bound to a volume served
// from host, and waits on the mount.
func nfsConsumer(f *fixture, host string) inventory.EntityID {
	pods := f.workload("shop", "api", 1)
	claim := inventory.CoreID(kube.KindPVC, "shop", "data")
	volume := inventory.CoreID(kube.KindPV, "", "data-pv")
	f.add(claim)
	f.observe(volume, map[string]inventory.Value{
		kube.AttrNFSServer: inventory.Text(host)})
	f.relate(pods[0], inventory.Mounts, claim)
	f.relate(claim, inventory.References, volume)
	f.fail(pods[0], modeFailedMount, failingH, 2,
		"mount.nfs: Connection timed out")
	return pods[0]
}

func TestMountOfServiceVolumeNeverBlamesTheAddress(t *testing.T) {
	f := newFixture(t)
	service, _, _ := redisFixture(f, 0, 0)
	effect := nfsConsumer(f, "redis.shop.svc.cluster.local")
	requireNotBlamed(t, f.explain(), effect, service)
}

func TestMountOfServiceVolumeBlamesTheFailingBackends(t *testing.T) {
	f := newFixture(t)
	svc, _, cache := redisFixture(f, 1, 0)
	f.fail(svc, "NoEndpoints", failingH, 2, "")
	f.fail(cache[0], "NotReady", failingH, 1, "")
	effect := nfsConsumer(f, "redis.shop.svc.cluster.local")
	c := requireCause(t, f.explain(), effect, "deployment/shop/redis")
	if c.Row != RuleCalledServiceBackendsFailing {
		t.Fatalf("row = %s", c.Row)
	}
}

func TestMountOfExternalVolumeNamesNoService(t *testing.T) {
	f := newFixture(t)
	redisFixture(f, 0, 0)
	effect := nfsConsumer(f, "nas.example.com")
	requireNotBlamed(t, f.explain(), effect,
		inventory.CoreID(kube.KindService, "shop", "redis"))
}
