package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const gadgetGroup = "gadgets.acme.dev"

// writeAccess lets the account update the API group, through a
// ClusterRole and a ClusterRoleBinding.
func (f *fixture) writeAccess(account inventory.EntityID, group string) {
	role := inventory.CoreID(kube.KindClusterRole, "", account.Name)
	f.add(role)
	f.setAttrs(role, attrs{kube.AttrRBACGroups: inventory.Text(group)})
	binding := inventory.CoreID(kube.KindClusterRoleBinding, "",
		account.Name)
	f.add(binding)
	f.setAttrs(binding, attrs{
		kube.AttrRBACRole: inventory.Text("ClusterRole/" + account.Name),
		kube.AttrRBACAccounts: inventory.Text(
			account.Namespace + "/" + account.Name)})
}

// controllerCase crash-loops a controller whose pod runs as an account
// that may write granted, and fails a Gadget of the custom group. The
// resource's field manager names no workload.
func controllerCase(
	f *fixture, granted string,
) (controller, gadget inventory.EntityID) {
	nodes := f.nodes("zone-a", "n1")
	pods := f.workload("gadget-system", "ctl", 1, nodes...)
	controller = inventory.CoreID(kube.KindDeployment, "gadget-system", "ctl")
	account := inventory.CoreID(kube.KindAccount, "gadget-system", "ctl")
	f.add(account)
	f.relate(pods[0], inventory.References, account)
	f.writeAccess(account, granted)
	f.fail(containerOf(pods[0]), detection.ModeCrashLoop, failingH, 1,
		panicText)
	gadget = inventory.NewEntityID(gadgetGroup, "gadget", "team-a", "main")
	f.add(gadget)
	f.setAttrs(gadget, attrs{kube.AttrManagedBy: inventory.Text("manager")})
	f.fail(gadget, detection.ModeCondition, degradedH, 3, "")
	return controller, gadget
}

// The workload whose account may write the resource's API group is the
// controller of the resource, whatever its field manager says.
func TestControllerGrantsLinkACustomResourceToItsWriter(t *testing.T) {
	f := newFixture(t)
	controller, gadget := controllerCase(f, gadgetGroup)

	c := requireCause(t, f.explain(), gadget, controller.String())
	if c.Row != "controller-failing-condition" {
		t.Fatalf("row = %s, want controller-failing-condition", c.Row)
	}
}

// A grant for another group says nothing about this resource.
func TestControllerGrantsIgnoreAnotherGroup(t *testing.T) {
	f := newFixture(t)
	controller, gadget := controllerCase(f, "other.acme.dev")

	if c, ok := f.explain().CauseOf(gadget); ok && c.Root == controller {
		t.Fatalf("a controller of another group blamed: %+v", c)
	}
}

// A healthy writer is no cause: the link alone proves nothing.
func TestControllerGrantsHealthyWriterIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	controller, gadget := controllerCase(f, gadgetGroup)
	delete(f.findings, containerOf(inventory.CoreID(kube.KindPod,
		"gadget-system", "ctl-1-0")))

	if c, ok := f.explain().CauseOf(gadget); ok && c.Root == controller {
		t.Fatalf("a healthy controller blamed: %+v", c)
	}
}

var controllerRowCases = []rowCase{
	{row: "controller-failing-condition", want: "deployment/gadget-system/ctl",
		build: func(f *fixture) inventory.EntityID {
			_, gadget := controllerCase(f, gadgetGroup)
			return gadget
		}},
}

// A route of the Gateway API is a custom resource like any other: the
// workload whose account may update its group is the gateway controller,
// and its failure explains the routes that report a failed condition.
func TestControllerGrantsLinkAGatewayRouteToItsController(t *testing.T) {
	f := newFixture(t)
	const group = "gateway.networking.k8s.io"
	controller, _ := controllerCase(f, group)
	route := inventory.NewEntityID(group, "httproute", "shop", "web")
	f.add(route)
	f.fail(route, detection.ModeCondition, degradedH, 3, "")

	requireCause(t, f.explain(), route, controller.String())
}
