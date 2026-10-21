package explain

import (
	"fmt"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var answeringDB = inventory.CoreID(kube.KindExternalEndpoint, "",
	"db.example.com:5432")

// The endpoint's own probe connects: it is not what the pods that time
// out on it fail on.
func TestEndpointThatAnswersTheProbeIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	effects := callStorm(f, 3, refusedDB)
	f.observe(answeringDB, map[string]inventory.Value{
		kube.AttrHealthy: inventory.Bool(true)})

	if c, ok := f.explain().CauseOf(effects[0]); ok &&
		c.Root == answeringDB {
		t.Fatalf("an endpoint that answers the probe blamed: %+v", c)
	}
}

// A probe that fails leaves the endpoint blamed, as before.
func TestEndpointThatFailsTheProbeStaysBlamed(t *testing.T) {
	f := newFixture(t)
	effects := callStorm(f, 3, refusedDB)
	f.observe(answeringDB, map[string]inventory.Value{
		kube.AttrHealthy: inventory.Bool(false)})

	requireCause(t, f.explain(), effects[0], answeringDB.String())
}

// nodeLocalStorm fails the replica of three workloads on n3 on the
// call while their replicas on n1 and n2 run; failingElsewhere also
// fails the replica on n1 of the first two workloads.
func nodeLocalStorm(
	f *fixture, failingElsewhere bool,
) []inventory.EntityID {
	nodes := f.nodes("zone-a", "n1", "n2", "n3")
	var out []inventory.EntityID
	for i := 0; i < 3; i++ {
		pods := f.workload("shop", fmt.Sprintf("app%d", i), 3, nodes...)
		f.ready(pods[0], pods[1])
		f.failError(containerOf(pods[2]), "CrashLoop", refusedDB)
		out = append(out, containerOf(pods[2]))
		if failingElsewhere && i < 2 {
			f.failError(containerOf(pods[0]), "CrashLoop", refusedDB)
		}
	}
	return out
}

// Pods that fail on one node while the same workloads' replicas call
// the endpoint from other nodes without a failure point at the node.
func TestEndpointFailuresOnOneNodeAreNotTheEndpoints(t *testing.T) {
	f := newFixture(t)
	effects := nodeLocalStorm(f, false)

	if c, ok := f.explain().CauseOf(effects[0]); ok &&
		c.Root == answeringDB {
		t.Fatalf("an endpoint blamed for one node's failures: %+v", c)
	}
}

// Workloads that fail on another node too show the endpoint is not
// reached from there either.
func TestEndpointFailuresOnTwoNodesStayTheEndpoints(t *testing.T) {
	f := newFixture(t)
	effects := nodeLocalStorm(f, true)

	requireCause(t, f.explain(), effects[0], answeringDB.String())
}
