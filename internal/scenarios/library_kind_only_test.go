package scenarios

import (
	"time"
)

// kindOnlyScenarios are storms where some callers name the address
// they cannot reach and others only say what kind of service it is.
func kindOnlyScenarios() []scenario {
	return []scenario{kindOnlyJoinsAddress(), kindOnlyOtherKind()}
}

// cacheAddressLine names the cache's address; cacheKindLine only says
// it is a cache (the client and its exception say Redis).
const (
	cacheAddressLine = "dial tcp 10.0.7.20:6379: connect: connection " +
		"refused"
	cacheKindLine = "StackExchange.Redis.RedisConnectionException: It " +
		"was not possible to connect to the redis server(s)"
	databaseKindLine = "fatal: could not connect to the database: " +
		"connection timed out"
)

// kindOnlyJoinsAddress: three services crash naming the cache's address,
// two crash at the same time saying only that the cache cannot be
// reached. One incident, named by the address, holds all five; the two
// are the same failure, address not stated.
func kindOnlyJoinsAddress() scenario {
	return scenario{
		expect: expectation{
			Name: "kind-only-joins-address",
			Description: "Three services name the cache address they " +
				"cannot reach, two only say the cache cannot be " +
				"reached: one incident named by the address.",
			Root: "external-endpoint//10.0.7.20:6379", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"node//n1", "deployment/shop/pricing",
				"deployment/shop/tracking"},
		},
		build: func(c *cluster) {
			buildKindStorm(c, cacheKindLine)
		},
	}
}

// kindOnlyOtherKind: the same three name the cache's address, while two
// others say they cannot reach their database. A database is not the
// cache: the two are not attached to the cache's incident.
func kindOnlyOtherKind() scenario {
	return scenario{
		expect: expectation{
			Name: "kind-only-other-kind",
			Description: "Three services name the cache address they " +
				"cannot reach; two others cannot reach their database " +
				"and are not attached to the cache.",
			Root: "external-endpoint//10.0.7.20:6379", Tier: "notify",
			OtherRoots: []string{"deployment/shop/pricing",
				"deployment/shop/tracking"},
			MaxMessages:  4,
			MustNotBlame: []string{"node//n1"},
		},
		build: func(c *cluster) {
			buildKindStorm(c, databaseKindLine)
		},
	}
}

// buildKindStorm crashes accounts, billing and fleet with the cache's
// address, and pricing and tracking with other, at the same time.
func buildKindStorm(c *cluster, other string) {
	c.list(c.node("n1", "zone-a"))
	lines := map[string]string{"accounts": cacheAddressLine,
		"billing": cacheAddressLine, "fleet": cacheAddressLine,
		"pricing": other, "tracking": other}
	var fleet []*workload
	for _, app := range []string{"accounts", "billing", "fleet",
		"pricing", "tracking"} {
		w := c.deployment("shop", app, "registry.example.com/"+app+":3", 1)
		c.list(w.objects())
		c.list(w.pod(0, "n1"))
		fleet = append(fleet, w)
	}
	c.after(5 * time.Minute)
	for restarts := int32(1); restarts <= 5; restarts++ {
		for _, w := range fleet {
			c.update(w.pod(0, "n1", crashLoop(1, "Error",
				lines[w.deployment.Name], restarts)))
			w.setReady(0)
			c.update(w.deployment, w.replicaSet)
		}
		c.after(time.Minute)
	}
	c.after(5 * time.Minute)
}
