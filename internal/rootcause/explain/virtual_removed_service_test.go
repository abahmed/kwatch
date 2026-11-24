package explain

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// removedCacheLines are three clients failing to reach Service cache of
// namespace data, each in its own words.
var removedCacheLines = []string{
	"StackExchange.Redis.RedisConnectionException: It was not possible " +
		"to connect to the redis server(s). UnableToConnect on " +
		"cache.data:6379/Interactive",
	"Error: Redis connection to cache.data:6379 failed - getaddrinfo " +
		"ENOTFOUND cache.data",
	"dial tcp: lookup cache.data.svc.cluster.local: no such host",
}

// removedFixture deletes Service cache of namespace data deletedAt
// minutes after t0, and has callers single-replica workloads that fail
// two minutes after t0, each quoting one of removedCacheLines. It
// returns the first caller's container.
func removedFixture(f *fixture, callers, deletedAt int) inventory.EntityID {
	f.add(inventory.CoreID(kube.KindNamespace, "", "data"),
		inventory.CoreID(kube.KindNamespace, "", "shop"))
	cache := inventory.CoreID(kube.KindService, "data", "cache")
	f.changes[cache] = append(f.changes[cache], inventory.Change{
		Entity: cache, Deleted: true,
		At: t0.Add(time.Duration(deletedAt) * time.Minute)})
	var first inventory.EntityID
	for i := range callers {
		pods := f.workload("shop", fmt.Sprintf("app-%d", i), 1)
		f.failError(containerOf(pods[0]), detection.ModeProbeReadiness,
			removedCacheLines[i%len(removedCacheLines)])
		if i == 0 {
			first = containerOf(pods[0])
		}
	}
	return first
}

var removedServiceRowCases = []rowCase{
	{row: RuleDependencyRemoved, want: "service/data/cache",
		build: func(f *fixture) inventory.EntityID {
			return removedFixture(f, 3, 1)
		}},
}

// Three workloads that began to fail a minute after the deletion, each
// naming the cache its own way: one cause, the removed Service.
func TestRemovedServiceIsTheOneCauseOfItsUsers(t *testing.T) {
	f := newFixture(t)
	removedFixture(f, 3, 1)
	e := f.explain()
	if got := causes(e); len(got) != 1 || got[0] != "service/data/cache" {
		t.Fatalf("causes = %v, want only the removed Service", got)
	}
}

// Two users are not yet a pattern, and a deletion far from when the
// users began to fail did not cause it: the removal row stays out,
// though the missing Service may still be named.
func TestRemovedServiceNeedsUsersAndTiming(t *testing.T) {
	for name, build := range map[string]func(*fixture) inventory.EntityID{
		"two users": func(f *fixture) inventory.EntityID {
			return removedFixture(f, 2, 1)
		},
		"deleted long before": func(f *fixture) inventory.EntityID {
			return removedFixture(f, 3, -8)
		},
	} {
		f := newFixture(t)
		effect := build(f)
		for _, area := range f.explain().Areas {
			for _, c := range area.Causes {
				if c.Row == RuleDependencyRemoved {
					t.Errorf("%s: %v blamed by %s", name, effect,
						c.Row)
				}
			}
		}
	}
}

// "cache.data" in a cluster with a namespace data is a Service name,
// never a host outside the cluster.
func TestServiceNameOfANamespaceIsInCluster(t *testing.T) {
	f := newFixture(t)
	f.add(inventory.CoreID(kube.KindNamespace, "", "data"))
	v := newView(f.snapshot())
	pod := inventory.CoreID(kube.KindPod, "shop", "app-0")
	if !v.inCluster("cache.data", pod) {
		t.Fatal("cache.data was read as a host outside the cluster")
	}
	if v.inCluster("cache.example", pod) {
		t.Fatal("cache.example was read as a Service")
	}
}
