package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// tier is one workload of a chain with its pods and Service.
type tier struct {
	name    string
	pods    []inventory.EntityID
	service inventory.EntityID
}

// chainTier adds a workload of two pods that calls callee (empty: none),
// and its Service with no ready endpoint. The pods are not ready.
func chainTier(
	f *fixture, name, callee string, ready bool, nodes ...inventory.EntityID,
) tier {
	pods := f.workload("shop", name, 2, nodes...)
	service := inventory.CoreID(kube.KindService, "shop", name)
	slice := inventory.CoreID(kube.KindEndpointSlice, "shop", name+"-x")
	f.add(service)
	count := 0.0
	if ready {
		count = 2
	}
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: slice,
		Attributes: map[string]inventory.Value{
			kube.AttrEndpoints:      inventory.Number(2),
			kube.AttrEndpointsReady: inventory.Number(count)}})
	f.relate(slice, inventory.Backs, service)
	for _, pod := range pods {
		attrs := map[string]inventory.Value{
			kube.AttrReady: inventory.Bool(ready)}
		if callee != "" {
			attrs[kube.AttrServiceCalls] = inventory.Text(
				"shop/" + callee + ":5432")
		}
		f.apply(inventory.Observation{Kind: inventory.Observed, Entity: pod,
			Attributes: attrs})
		f.links[service] = append(f.links[service],
			Link{Type: inventory.Selects, To: pod})
	}
	return tier{name: name, pods: pods, service: service}
}

// failTier gives the tier the failures a readiness cascade shows: its
// containers are not ready and its Service has no endpoints.
func failTier(f *fixture, t tier, minutes int) {
	for _, pod := range t.pods {
		f.fail(containerOf(pod), detection.ModeNotReady, failingH, minutes, "")
	}
	f.fail(t.service, detection.ModeNoEndpoints, failingH, minutes, "")
}

// chainNode is a node that stops reporting, with the database on it: the
// cause the chain tests extend. It returns the node and the database.
func chainNode(f *fixture, minutes int) (inventory.EntityID, tier) {
	nodes := f.nodes("zone-a", "n1", "n2")
	db := chainTier(f, "postgres", "", false, nodes[0])
	f.fail(nodes[0], detection.ModeNotReady, failingH, minutes, "")
	failTier(f, db, minutes+1)
	return nodes[0], db
}

func hopNames(hops []Hop) []string {
	var out []string
	for _, h := range hops {
		out = append(out, h.Entity.String())
	}
	return out
}

func requireHops(t *testing.T, c Cause, want ...string) {
	t.Helper()
	got := hopNames(c.Hops)
	if len(got) != len(want) {
		t.Fatalf("hops = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("hops = %v, want %v", got, want)
		}
	}
}

func TestChainExtendsACauseAlongTheFailureChain(t *testing.T) {
	f := newFixture(t)
	node, _ := chainNode(f, 1)
	nodes := f.nodes("zone-a", "n3")
	api := chainTier(f, "api", "postgres", false, nodes[0])
	failTier(f, api, 3)

	e := f.explain()

	c := requireCause(t, e, containerOf(api.pods[0]), "node//n1")
	requireHops(t, c, node.String(), "deployment/shop/postgres",
		"deployment/shop/api", "service/shop/api")
	if _, ok := e.CauseOf(api.service); !ok {
		t.Fatal("the Service behind the API must be covered too")
	}
	if got := causes(e); len(got) != 1 {
		t.Fatalf("causes = %v, want the node alone", got)
	}
}

func TestChainConfidenceDecaysAtEveryStep(t *testing.T) {
	f := newFixture(t)
	chainNode(f, 1)
	api := chainTier(f, "api", "postgres", false, f.nodes("z", "n3")[0])
	failTier(f, api, 3)

	c := requireCause(t, f.explain(), containerOf(api.pods[0]), "node//n1")

	if c.Hops[0].Confidence != c.Confidence {
		t.Fatalf("root confidence = %v, want the cause's %v",
			c.Hops[0].Confidence, c.Confidence)
	}
	for i := 2; i < len(c.Hops); i++ {
		want := c.Hops[i-1].Confidence * ChainDecay
		if diff := c.Hops[i].Confidence - want; diff > 1e-9 || diff < -1e-9 {
			t.Fatalf("hop %d confidence = %v, want %v", i,
				c.Hops[i].Confidence, want)
		}
	}
}

func TestChainStopsAtThreeSteps(t *testing.T) {
	f := newFixture(t)
	chainNode(f, 1)
	callee := "postgres"
	var last tier
	for i, name := range []string{"api", "gateway", "web", "edge"} {
		last = chainTier(f, name, callee, false, f.nodes("z", "x"+name)[0])
		failTier(f, last, 3+i)
		callee = name
	}

	e := f.explain()

	c := requireCause(t, e, containerOf(inventory.CoreID(kube.KindPod, "shop",
		"gateway-1-0")), "node//n1")
	if len(c.Hops) != 1+1+ChainMaxHops {
		t.Fatalf("hops = %v, want the node, the database and %d steps",
			hopNames(c.Hops), ChainMaxHops)
	}
	if len(c.Beyond) == 0 {
		t.Fatal("the failure past the limit must be named")
	}
	if cause, ok := e.CauseOf(containerOf(last.pods[0])); ok &&
		cause.Root.String() == "node//n1" {
		t.Fatalf("the fourth step %s must not be claimed", last.name)
	}
}

func TestChainNeedsTheCauseToBeginFirst(t *testing.T) {
	f := newFixture(t)
	chainNode(f, 5)
	api := chainTier(f, "api", "postgres", false, f.nodes("z", "n3")[0])
	failTier(f, api, 2)

	e := f.explain()

	if c, ok := e.CauseOf(containerOf(api.pods[0])); ok &&
		c.Root.String() == "node//n1" {
		t.Fatalf("the API began before the node: it is not a consequence")
	}
	for _, c := range e.Areas {
		for _, cause := range c.Causes {
			if len(cause.Hops) > 1 {
				t.Fatalf("hops = %v, want none", hopNames(cause.Hops))
			}
		}
	}
}

func TestChainStopsAtAServiceWithEndpoints(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	db := chainTier(f, "postgres", "", true, nodes[0])
	f.fail(nodes[0], detection.ModeNotReady, failingH, 1, "")
	f.fail(containerOf(db.pods[0]), detection.ModeNotReady, failingH, 2, "")
	api := chainTier(f, "api", "postgres", false, nodes[1])
	failTier(f, api, 4)

	e := f.explain()

	if c, ok := e.CauseOf(containerOf(api.pods[0])); ok &&
		c.Root.String() == "node//n1" {
		t.Fatal("postgres still has a ready endpoint: the call is not down")
	}
}

func TestChainNeverPassesThroughAHealthyStep(t *testing.T) {
	f := newFixture(t)
	chainNode(f, 1)
	healthy := chainTier(f, "api", "postgres", true, f.nodes("z", "n3")[0])
	web := chainTier(f, "web", "api", false, f.nodes("z", "n4")[0])
	failTier(f, web, 4)
	_ = healthy

	e := f.explain()

	if c, ok := e.CauseOf(containerOf(web.pods[0])); ok &&
		c.Root.String() == "node//n1" {
		t.Fatal("the API is healthy: the node cannot reach the web tier")
	}
}

func TestChainLeavesAWorkloadWithAChangeOfItsOwn(t *testing.T) {
	f := newFixture(t)
	chainNode(f, 1)
	api := chainTier(f, "api", "postgres", false, f.nodes("z", "n3")[0])
	failTier(f, api, 3)
	f.change(inventory.CoreID(kube.KindDeployment, "shop", "api"), 2,
		"spec.template.spec.containers[0].image")

	e := f.explain()

	if c, ok := e.CauseOf(containerOf(api.pods[0])); ok &&
		c.Root.String() == "node//n1" {
		t.Fatal("the API was changed just before it failed: not the node's")
	}
}

func TestChainLeavesAPodWithAnotherFailure(t *testing.T) {
	f := newFixture(t)
	chainNode(f, 1)
	api := chainTier(f, "api", "postgres", false, f.nodes("z", "n3")[0])
	failTier(f, api, 3)
	f.fail(containerOf(api.pods[0]), detection.ModeOOMKilled, failingH, 3, "")

	e := f.explain()

	if c, ok := e.CauseOf(containerOf(api.pods[0])); ok &&
		c.Root.String() == "node//n1" {
		t.Fatal("an OOM kill is not a failed call")
	}
}

func TestChainFollowsAFocusedFailureBackToItsCause(t *testing.T) {
	f := newFixture(t)
	chainNode(f, 1)
	api := chainTier(f, "api", "postgres", false, f.nodes("z", "n3")[0])
	failTier(f, api, 3)
	s := f.snapshot()
	s.Focus = []inventory.EntityID{containerOf(api.pods[0])}

	e := Explain(s)

	requireCause(t, e, containerOf(api.pods[0]), "node//n1")
}

func TestChainReachesAWorkloadThatShowsNoFindingYet(t *testing.T) {
	f := newFixture(t)
	chainNode(f, 1)
	api := chainTier(f, "api", "postgres", false, f.nodes("z", "n3")[0])
	// Only the Service shows it: the detectors still wait on the pods.
	f.fail(api.service, detection.ModeNoEndpoints, failingH, 3, "")
	// The pods were ready until minute 3, so they stopped being ready
	// after the node failed.
	for i, ready := range []bool{true, false} {
		for _, pod := range api.pods {
			if _, err := f.model.Apply(inventory.Observation{
				Kind: inventory.Observed, Source: "test",
				At:     t0.Add(3*time.Minute + time.Duration(i)*time.Second),
				Entity: pod, Attributes: map[string]inventory.Value{
					kube.AttrReady: inventory.Bool(ready),
					kube.AttrServiceCalls: inventory.Text(
						"shop/postgres:5432")}}); err != nil {
				t.Fatal(err)
			}
		}
	}

	e := f.explain()

	c := requireCause(t, e, api.service, "node//n1")
	requireHops(t, c, "node//n1", "deployment/shop/postgres",
		"deployment/shop/api", "service/shop/api")
}

func TestChainReachesTheRouteThatSendsTrafficToTheLastService(t *testing.T) {
	f := newFixture(t)
	chainNode(f, 1)
	api := chainTier(f, "api", "postgres", false, f.nodes("z", "n3")[0])
	failTier(f, api, 3)
	ingress := inventory.CoreID(kube.KindIngress, "shop", "shop")
	f.add(ingress)
	f.relate(ingress, inventory.RoutesTo, api.service)
	f.fail(ingress, detection.ModeNotReconciling, failingH, 4, "")

	e := f.explain()

	c := requireCause(t, e, ingress, "node//n1")
	requireHops(t, c, "node//n1", "deployment/shop/postgres",
		"deployment/shop/api", "service/shop/api", "ingress/shop/shop")
}
