package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// addressFixture is policyFixture without the configuration that names
// the Service: the API only knows the address, in its error.
func addressFixture(
	f *fixture, serviceIP, podIP, dialed string,
) []inventory.EntityID {
	svc := inventory.CoreID(kube.KindService, "data", "db")
	dbPod := inventory.CoreID(kube.KindPod, "data", "db-0")
	svcAttrs := map[string]inventory.Value{
		kube.AttrSelector: inventory.Text("app=db"),
		kube.AttrPorts:    inventory.Text("5432/TCP->5432")}
	if serviceIP != "" {
		svcAttrs[kube.AttrClusterIP] = inventory.Text(serviceIP)
	}
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: svc,
		Attributes: svcAttrs})
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: dbPod,
		Attributes: map[string]inventory.Value{
			kube.AttrLabels: inventory.Text("app=db"),
			kube.AttrPodIP:  inventory.Text(podIP)}})
	f.links[svc] = []Link{{Type: inventory.Selects, To: dbPod}}
	api := f.workload("shop", "api", 2)
	for _, pod := range api {
		f.failError(containerOf(pod), "CrashLoop", dialed)
	}
	return api
}

func TestPolicyBlocksACallDialedByServiceAddress(t *testing.T) {
	f := newFixture(t)
	api := addressFixture(f, "10.96.14.3", "10.244.1.9",
		"dial tcp 10.96.14.3:5432: i/o timeout")
	policy := addPolicy(f, "shop", "deny-all", denyEgress)
	f.change(policy, 1)

	c := requireRoot(t, f, containerOf(api[0]), policy.String())

	if c.Row != "policy-blocks-call" ||
		c.Mode != ModeBlocksCall+".db:5432" {
		t.Fatalf("row %q mode %q", c.Row, c.Mode)
	}
}

func TestPolicyBlocksACallDialedByPodAddress(t *testing.T) {
	f := newFixture(t)
	api := addressFixture(f, "10.96.14.3", "10.244.1.9",
		"dial tcp 10.244.1.9:5432: i/o timeout")
	policy := addPolicy(f, "shop", "deny-all", denyEgress)
	f.change(policy, 1)

	requireRoot(t, f, containerOf(api[0]), policy.String())
}

// An address the model does not hold names no Service: nothing is
// guessed, so the policy is not blamed for it.
func TestPolicyIsNotBlamedForAnUnknownAddress(t *testing.T) {
	f := newFixture(t)
	api := addressFixture(f, "10.96.14.3", "10.244.1.9",
		"dial tcp 10.99.99.9:5432: i/o timeout")
	policy := addPolicy(f, "shop", "deny-all", denyEgress)
	f.change(policy, 1)

	if c, ok := f.explain().CauseOf(containerOf(api[0])); ok &&
		c.Root == policy {
		t.Fatalf("blamed a policy for an address nothing owns: %+v", c)
	}
}

// The address is the Service's only for the port it serves.
func TestPodAddressNeedsTheServicePort(t *testing.T) {
	f := newFixture(t)
	api := addressFixture(f, "", "10.244.1.9",
		"dial tcp 10.244.1.9:9999: i/o timeout")
	policy := addPolicy(f, "shop", "deny-all", denyEgress)
	f.change(policy, 1)

	if c, ok := f.explain().CauseOf(containerOf(api[0])); ok &&
		c.Root == policy {
		t.Fatalf("a port the Service does not serve resolved: %+v", c)
	}
}
