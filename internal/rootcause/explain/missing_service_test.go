package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

const unknownPaymnts = "dial tcp: lookup paymnts.shop.svc.cluster.local: " +
	"no such host"

// missingFixture is an API whose pods call Service paymnts, a name no
// Service has; the namespace has a Service named payments.
func missingFixture(
	f *fixture, calls string,
) (api []inventory.EntityID, missing inventory.EntityID) {
	f.add(inventory.CoreID(kube.KindNamespace, "", "shop"),
		inventory.CoreID(kube.KindService, "shop", "payments"))
	api = f.workload("shop", "api", 2)
	if calls != "" {
		for _, pod := range api {
			f.apply(inventory.Observation{Kind: inventory.Observed,
				Entity: pod, Attributes: map[string]inventory.Value{
					kube.AttrServiceCalls: inventory.Text(calls)}})
		}
	}
	return api, inventory.CoreID(kube.KindService, "shop", "paymnts")
}

var missingServiceRowCases = []rowCase{
	{row: "missing-service-called", want: "service/shop/paymnts",
		build: func(f *fixture) inventory.EntityID {
			api, _ := missingFixture(f, "")
			failAPI(f, api, unknownPaymnts)
			return containerOf(api[0])
		}},
	{row: "missing-service-configured", want: "service/shop/paymnts",
		build: func(f *fixture) inventory.EntityID {
			api, _ := missingFixture(f, "shop/paymnts:8080")
			failAPI(f, api, "panic: configuration invalid")
			return containerOf(api[0])
		}},
}

func TestMissingServiceIsTheRootOfAQuotedLookup(t *testing.T) {
	f := newFixture(t)
	api, missing := missingFixture(f, "shop/paymnts:8080")
	failAPI(f, api, unknownPaymnts)

	c := requireRoot(t, f, containerOf(api[0]), missing.String())

	if c.Row != "missing-service-called" {
		t.Fatalf("row = %q", c.Row)
	}
	var proof *Contribution
	for i := range c.Contributions {
		if c.Contributions[i].Code == rootcause.ProofMissingCall {
			proof = &c.Contributions[i]
		}
	}
	if proof == nil || proof.Count != 8080 ||
		len(proof.Fields) != 1 || proof.Fields[0] != "payments" {
		t.Fatalf("proof = %+v, want port 8080 and the name payments",
			proof)
	}
}

func TestMissingServiceFromTheLookupAloneHasNoPort(t *testing.T) {
	f := newFixture(t)
	api, missing := missingFixture(f, "")
	failAPI(f, api, unknownPaymnts)

	requireRoot(t, f, containerOf(api[0]), missing.String())
}

func TestMissingServiceFromConfigWhenTheWorkloadCrashes(t *testing.T) {
	f := newFixture(t)
	api, missing := missingFixture(f, "shop/paymnts:8080")
	failAPI(f, api, "panic: configuration invalid")

	c := requireRoot(t, f, containerOf(api[0]), missing.String())

	if c.Row != "missing-service-configured" {
		t.Fatalf("row = %q", c.Row)
	}
}

func TestMissingServiceIsQuietWhenNothingFails(t *testing.T) {
	f := newFixture(t)
	missingFixture(f, "shop/paymnts:8080")

	if got := causes(f.explain()); len(got) != 0 {
		t.Fatalf("causes = %v, want none for healthy pods", got)
	}
}

func TestExistingServiceIsNotMissing(t *testing.T) {
	f := newFixture(t)
	api, _ := missingFixture(f, "shop/payments:8080")
	failAPI(f, api, "panic: configuration invalid")

	for _, root := range causes(f.explain()) {
		if root == "service/shop/payments" {
			t.Fatalf("an existing Service was blamed: %v", root)
		}
	}
}

func TestMissingServiceWaitsForServicesToSync(t *testing.T) {
	f := newFixture(t)
	api, missing := missingFixture(f, "shop/paymnts:8080")
	failAPI(f, api, unknownPaymnts)
	snapshot := f.snapshot()
	snapshot.Synced = func(kind inventory.Kind) bool {
		return kind != kube.KindService
	}

	e := Explain(snapshot)

	for _, root := range causes(e) {
		if root == missing.String() {
			t.Fatalf("a Service not yet listed was called missing")
		}
	}
}

func TestLookupOfAnExternalNameIsNotAMissingService(t *testing.T) {
	f := newFixture(t)
	api, _ := missingFixture(f, "")
	failAPI(f, api, "dial tcp: lookup db.example: no such host")

	for _, root := range causes(f.explain()) {
		if root == "service/example/db" {
			t.Fatalf("an external name was read as a Service: %v", root)
		}
	}
}

func TestClosestServiceNameIsOnlyVeryClose(t *testing.T) {
	names := []string{"payments", "orders", "api"}
	cases := map[string]string{"paymnts": "payments", "ordrs": "orders",
		"inventory": "", "apx": "", "cart": ""}
	for missing, want := range cases {
		if got := closestName(missing, names); got != want {
			t.Errorf("closestName(%q) = %q, want %q", missing, got, want)
		}
	}
	if got := closestName("pay", []string{"paw", "pad"}); got != "" {
		t.Errorf("a tie suggested %q", got)
	}
}

func TestMissingServiceYieldsToTheRolloutThatNamedIt(t *testing.T) {
	f := newFixture(t)
	api, missing := missingFixture(f, "shop/paymnts:8080")
	failAPI(f, api, unknownPaymnts)
	f.change(inventory.CoreID(kube.KindDeployment, "shop", "api"), 1,
		"spec.template.spec.containers[0].env")

	for _, root := range causes(f.explain()) {
		if root == missing.String() {
			t.Fatalf("the missing Service outranked the change that set it")
		}
	}
}
