package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// serviceAttrs sets attributes on the cache Service of redisFixture.
func serviceAttrs(f *fixture, attrs map[string]inventory.Value) {
	f.apply(inventory.Observation{Kind: inventory.Observed,
		Entity:     inventory.CoreID(kube.KindService, "shop", "redis"),
		Attributes: attrs})
}

var serviceDefectRowCases = []rowCase{
	{row: RuleServiceProtocolMismatch, want: "service/shop/redis",
		build: func(f *fixture) inventory.EntityID {
			_, api, _ := redisFixture(f, 0, 0)
			serviceAttrs(f, map[string]inventory.Value{
				kube.AttrPorts: inventory.Text("6379/UDP->6379")})
			failAPI(f, api, "dial tcp redis:6379: i/o timeout")
			return containerOf(api[0])
		}},
	{row: RuleServiceNameIsAddress, want: "service/shop/redis",
		build: func(f *fixture) inventory.EntityID {
			_, api, _ := redisFixture(f, 0, 0)
			serviceAttrs(f, map[string]inventory.Value{
				kube.AttrServiceType:  inventory.Text("ExternalName"),
				kube.AttrExternalName: inventory.Text("10.0.5.12")})
			failAPI(f, api, "dial tcp: lookup redis: no such host")
			return containerOf(api[0])
		}},
	{row: RuleServiceFamilyMismatch, want: "service/shop/redis",
		build: func(f *fixture) inventory.EntityID {
			_, api, cache := redisFixture(f, 1, 0)
			serviceAttrs(f, map[string]inventory.Value{
				kube.AttrClusterIP: inventory.Text("fd00:10:96::2a")})
			f.apply(inventory.Observation{Kind: inventory.Observed,
				Entity: cache[0], Attributes: map[string]inventory.Value{
					kube.AttrPodIP: inventory.Text("10.244.1.7")}})
			failAPI(f, api, "dial tcp [fd00:10:96::2a]:6379: "+
				"connect: connection refused")
			return containerOf(api[0])
		}},
}
