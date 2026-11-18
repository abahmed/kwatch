package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
)

// firstAnnounce is the facts of the first announcement of name.
func firstAnnounce(t *testing.T, name string) incident.Facts {
	t.Helper()
	for _, d := range replayNamed(t, name).Decisions {
		if d.Action == incident.Announce {
			return d.Facts
		}
	}
	t.Fatalf("%s announced nothing", name)
	return incident.Facts{}
}

// A grouped issue says what ties its members together; an issue of
// one workload says nothing of the kind.
func TestShapeSaysWhyMembersAreOneIssue(t *testing.T) {
	for name, want := range map[string]string{
		"node-pressure-evicted-linger": "Grouped because all 12 pods " +
			"run on node n1.",
		"readiness-outage-shared-database": "Grouped because all 7 " +
			"workloads call 10.0.9.40:5432.",
		"shared-panic-storm": "Grouped because 4 workloads log the " +
			"same error: panic: license check failed for tenant 23.",
		"change-blast-overlap": "Grouped because both workloads read " +
			"ConfigMap app-defaults.",
	} {
		t.Run(name, func(t *testing.T) {
			f := firstAnnounce(t, name)
			require.NotNil(t, f.Grouping)
			assert.Equal(t, want, f.Grouping.Text)
			assert.GreaterOrEqual(t, len(f.Grouping.Groups), 2)
		})
	}
	t.Run("one workload", func(t *testing.T) {
		assert.Nil(t, firstAnnounce(t, "replicas-fail-only-on-arm64").
			Grouping)
	})
}

// Only some replicas fail and one attribute splits them cleanly: the
// comparison says so. A mixed split, or every replica failing, says
// nothing.
func TestShapeComparesHealthyAndFailingReplicas(t *testing.T) {
	for name, want := range map[string][]string{
		"replicas-fail-only-on-arm64": {"Only the 3 pods on arm64 " +
			"nodes fail; the 4 pods on amd64 nodes are healthy."},
		"zone-separates-replicas": {"Only the 2 pods in zone zone-b " +
			"fail; the 2 pods in zone zone-a are healthy."},
		"same-tag-new-digest": {"Only the 2 pods running " +
			"registry.example.com/api:latest as sha256:9f1c0a2b44d1 " +
			"fail; the 2 pods running registry.example.com/api:latest " +
			"as sha256:aaaa1111bbbb are healthy."},
	} {
		t.Run(name, func(t *testing.T) {
			f := firstAnnounce(t, name)
			require.NotNil(t, f.Split)
			assert.Equal(t, want, f.Split.Lines)
		})
	}
	for _, name := range []string{"replicas-fail-on-mixed-nodes",
		"service-backends-crash"} {
		t.Run(name, func(t *testing.T) {
			assert.Nil(t, firstAnnounce(t, name).Split)
		})
	}
}

// Whether users are affected comes from the routes, Services and
// endpoints kwatch sees, and says what it does not see.
func TestShapeSaysWhetherUsersAreAffected(t *testing.T) {
	for name, want := range map[string]string{
		"user-endpoints-named": "Users are affected: route " +
			"shop.example.com/pay/refunds → payments has 0 of 2 ready " +
			"endpoints.",
		"loadbalancer-subnets-not-found": "Users are still served: " +
			"route shop.example.com → web has 2 of 2 ready endpoints.",
		"service-backends-crash": "Service payments has 0 of 2 ready " +
			"endpoints; kwatch does not see a route to it from outside " +
			"the cluster or another workload that calls it.",
		"replicas-fail-only-on-arm64": "kwatch does not see a Service " +
			"or a route in front of it.",
	} {
		t.Run(name, func(t *testing.T) {
			f := firstAnnounce(t, name)
			require.NotNil(t, f.Users)
			assert.Equal(t, want, f.Users.Text)
		})
	}
}
