package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Pods configured with two Services, one that has never existed and a
// cache whose backends went away just now, fail with the cache's own
// error. The name only the configuration holds, which the pods ran with
// before, is not the cause: their errors point at the cache.
func TestMissingConfiguredServiceYieldsToTheCallTheErrorsName(t *testing.T) {
	for name, text := range map[string]string{
		"client exception": "StackExchange.Redis.RedisConnectionException: " +
			"It was not possible to connect to the redis server(s)",
		"startup line": "fatal: cache redis unavailable at startup",
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			_, api, _ := redisFixture(f, 0, 0)
			for _, pod := range api {
				f.apply(inventory.Observation{Kind: inventory.Observed,
					Entity: pod, Attributes: map[string]inventory.Value{
						kube.AttrServiceCalls: inventory.Text(
							"shop/regional:8080,shop/redis:6379"),
					}})
			}
			failAPI(f, api, text)

			c, _ := f.explain().CauseOf(containerOf(api[0]))

			if c.Root == inventory.CoreID(kube.KindService, "shop",
				"regional") {
				t.Fatalf("blamed the long-missing Service: %s", c.Row)
			}
		})
	}
}

// A missing name the errors do name is still the cause.
func TestMissingConfiguredServiceStandsWhenTheErrorsNameIt(t *testing.T) {
	f := newFixture(t)
	api, missing := missingFixture(f, "shop/paymnts:8080,shop/redis:6379")
	failAPI(f, api, "panic: paymnts unavailable")

	requireRoot(t, f, containerOf(api[0]), missing.String())
}
