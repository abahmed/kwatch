package explain

import (
	"testing"
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const timedOutDB = "dial tcp 10.96.14.3:5432: i/o timeout"

// policyFixture is an API in shop configured to call Service db in
// data, whose pod runs there. The API pods fail with timedOutDB.
func policyFixture(f *fixture) (api []inventory.EntityID) {
	svc := inventory.CoreID(kube.KindService, "data", "db")
	dbPod := inventory.CoreID(kube.KindPod, "data", "db-0")
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: svc,
		Attributes: map[string]inventory.Value{
			kube.AttrSelector: inventory.Text("app=db"),
			kube.AttrPorts:    inventory.Text("5432/TCP->5432")}})
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: dbPod,
		Attributes: map[string]inventory.Value{
			kube.AttrLabels: inventory.Text("app=db")}})
	f.links[svc] = []Link{{Type: inventory.Selects, To: dbPod}}
	api = f.workload("shop", "api", 2)
	for _, pod := range api {
		f.apply(inventory.Observation{Kind: inventory.Observed, Entity: pod,
			Attributes: map[string]inventory.Value{
				kube.AttrServiceCalls: inventory.Text("data/db:5432")}})
		f.failError(containerOf(pod), "CrashLoop", timedOutDB)
	}
	return api
}

// addPolicy adds a NetworkPolicy of ns with spec and returns its id.
func addPolicy(
	f *fixture, ns, name string, spec networkingv1.NetworkPolicySpec,
) inventory.EntityID {
	d, _ := kube.NetworkPolicySchema{}.Describe(&networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       spec})
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: d.ID,
		Attributes: d.Attributes})
	return d.ID
}

var denyEgress = networkingv1.NetworkPolicySpec{
	PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
}

func TestPolicyBlocksCallIsTheRootWhenItChangedRecently(t *testing.T) {
	f := newFixture(t)
	api := policyFixture(f)
	policy := addPolicy(f, "shop", "deny-all", denyEgress)
	f.change(policy, 1)

	c := requireRoot(t, f, containerOf(api[0]), policy.String())

	if c.Row != "policy-blocks-call" ||
		c.Mode != ModeBlocksCall+".db:5432" {
		t.Fatalf("row %q mode %q", c.Row, c.Mode)
	}
}

func TestPolicyBlocksCallNeedsARecentChange(t *testing.T) {
	f := newFixture(t)
	api := policyFixture(f)
	policy := addPolicy(f, "shop", "deny-all", denyEgress)

	c, ok := f.explain().CauseOf(containerOf(api[0]))

	if ok && c.Root == policy {
		t.Fatalf("blamed a policy that did not change: %v", c.Root)
	}
}

func TestPolicyBlocksCallNeedsTheCallToBeBlocked(t *testing.T) {
	f := newFixture(t)
	api := policyFixture(f)
	spec := denyEgress
	spec.Egress = []networkingv1.NetworkPolicyEgressRule{{}}
	policy := addPolicy(f, "shop", "allow-all-egress", spec)
	f.change(policy, 1)

	c, ok := f.explain().CauseOf(containerOf(api[0]))

	if ok && c.Root == policy {
		t.Fatalf("blamed a policy that allows the call: %v", c.Root)
	}
}

func TestPolicyBlocksCallOnTheServicesSide(t *testing.T) {
	f := newFixture(t)
	api := policyFixture(f)
	policy := addPolicy(f, "data", "deny-ingress",
		networkingv1.NetworkPolicySpec{})
	f.change(policy, 1)

	requireRoot(t, f, containerOf(api[0]), policy.String())
}

func TestPolicyBlocksCallLeavesOtherFailuresAlone(t *testing.T) {
	f := newFixture(t)
	api := policyFixture(f)
	policy := addPolicy(f, "shop", "deny-all", denyEgress)
	f.change(policy, 1)
	other := f.workload("shop", "worker", 1)
	f.fail(other[0], detection.ModeCrashLoop, failingH, 2, "")

	c, ok := f.explain().CauseOf(other[0])

	if ok && c.Root == policy {
		t.Fatalf("blamed %v for a pod that calls nothing", policy)
	}
	requireRoot(t, f, containerOf(api[0]), policy.String())
}

func TestPolicyCreatedLaterSolvesTheAreaAgain(t *testing.T) {
	f := newFixture(t)
	policyFixture(f)
	solver := NewSolver()
	first := solver.Solve(f.snapshot(), nil)
	if got := causeRoots(first); len(got) != 1 || got[0].Kind == kube.
		KindNetworkPolicy {
		t.Fatalf("first solve = %v", got)
	}
	policy := addPolicy(f, "shop", "deny-all", denyEgress)
	f.change(policy, 1)
	f.now = f.now.Add(5 * time.Minute)

	again := solver.Solve(f.snapshot(), []inventory.EntityID{
		inventory.CoreID(kube.KindNamespace, "", "shop")})

	roots := causeRoots(again)
	if len(roots) != 1 || roots[0] != policy {
		t.Fatalf("roots = %v, want the new policy", roots)
	}
}

func causeRoots(areas []Area) []inventory.EntityID {
	var out []inventory.EntityID
	for _, a := range areas {
		for _, c := range a.Causes {
			out = append(out, c.Root)
		}
	}
	return out
}
