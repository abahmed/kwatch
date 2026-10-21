package explain

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

const (
	arcGroup     = "actions.github.com"
	arcFinalizer = "autoscalingrunnerset." + arcGroup + "/finalizer"
)

// set gives an entity attributes, replacing the ones the fixture set.
func (f *fixture) set(id inventory.EntityID, attrs map[string]inventory.Value) {
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: id,
		Attributes: attrs})
}

// stuckOf adds n objects of an API group and kind, in ns ("" for a
// cluster-scoped kind), whose deletion the finalizers hold.
func (f *fixture) stuckOf(
	group string, kind inventory.Kind, ns string, n int,
	finalizers ...string,
) []inventory.EntityID {
	var out []inventory.EntityID
	for i := 0; i < n; i++ {
		id := inventory.NewEntityID(group, kind, ns,
			string(kind)+"-"+string(rune('a'+i)))
		f.add(id)
		f.findings[id] = append(f.findings[id], detection.Finding{
			Entity: id, Reason: "StuckDeleting",
			Mode: detection.ModeStuckDeleting, Health: degradedH,
			Since:   t0.Add(-10 * 24 * time.Hour),
			Summary: "Deletion is blocked", Evidence: []detection.Evidence{
				{Label: "finalizers", Value: strings.Join(finalizers, ", ")}},
		})
		out = append(out, id)
	}
	return out
}

// runnerSet adds the leftovers of an ARC-like operator.
func (f *fixture) runnerSet() []inventory.EntityID {
	return f.stuckOf(arcGroup, "autoscalingrunnerset", "arc", 3,
		arcFinalizer)
}

// accountOf gives the workload a ServiceAccount and returns it.
func (f *fixture) accountOf(w inventory.EntityID) inventory.EntityID {
	account := inventory.CoreID(kube.KindAccount, w.Namespace, w.Name)
	f.add(account)
	f.relate(w, inventory.References, account)
	return account
}

// grant binds the account, by a ClusterRoleBinding, to a ClusterRole
// that lets it write the groups.
func (f *fixture) grant(account inventory.EntityID, groups string) {
	role := inventory.CoreID(kube.KindClusterRole, "", account.Name)
	f.set(role, map[string]inventory.Value{
		kube.AttrRBACGroups: inventory.Text(groups)})
	binding := inventory.CoreID(kube.KindClusterRoleBinding, "", account.Name)
	f.set(binding, map[string]inventory.Value{
		kube.AttrRBACRole: inventory.Text("ClusterRole/" + account.Name),
		kube.AttrRBACAccounts: inventory.Text(
			account.Namespace + "/" + account.Name)})
}

func notBlamed(t *testing.T, e Explanation, id inventory.EntityID,
	not ...inventory.EntityID,
) Cause {
	t.Helper()
	c, ok := e.CauseOf(id)
	if !ok || c.Root.Kind != KindFinalizer {
		t.Fatalf("cause of %s = %v (ok %v), want the finalizer; all %v",
			id, c.Root, ok, causes(e))
	}
	for _, w := range not {
		if c.Root == w {
			t.Fatalf("%s is blamed", w)
		}
	}
	return c
}

// Two scaled-down deployments named like the operator of a group are not
// its handler when what they may write is another group: names prove
// nothing.
func TestLookAlikeControllersAreNotBlamed(t *testing.T) {
	f := newFixture(t)
	ctl := f.controller(kube.KindDeployment, "arc",
		"actions-runner-controller", 0, 0, "app=arc")
	hook := f.controller(kube.KindDeployment, "arc",
		"actions-runner-controller-github-webhook-server", 0, 0, "app=arc")
	f.grant(f.accountOf(ctl), "actions.summerwind.dev")
	f.grant(f.accountOf(hook), "actions.summerwind.dev")
	stuck := f.runnerSet()

	c := notBlamed(t, f.explain(), stuck[0], ctl, hook)

	if c.Mode != ModeFinalizerUnhandled {
		t.Fatalf("mode = %q, want %q", c.Mode, ModeFinalizerUnhandled)
	}
	var hint rootcause.Proof
	for _, p := range recordProofs(c) {
		if p.Code == rootcause.ProofNamedLikeHandler {
			hint = p
		}
	}
	want := []string{"deployment arc/actions-runner-controller",
		"deployment arc/actions-runner-controller-github-webhook-server"}
	if len(hint.Fields) != 2 || hint.Fields[0] != want[0] ||
		hint.Fields[1] != want[1] {
		t.Fatalf("look-alike hint = %v, want %v", hint.Fields, want)
	}
	if hint.Supports || hint.Weight != 0 {
		t.Fatalf("a hint must not score: %+v", hint)
	}
}

func recordProofs(c Cause) []rootcause.Proof {
	var out []rootcause.Proof
	for _, k := range c.Contributions {
		out = append(out, rootcause.Proof{Code: k.Code, Fields: k.Fields,
			Weight: abs(k.Weight), Supports: k.Weight > 0})
	}
	return out
}

// The same controller, whose ServiceAccount may write the group, is
// the handler and is blamed while it is scaled to zero.
func TestControllerWhoseRoleWritesTheGroupIsBlamed(t *testing.T) {
	f := newFixture(t)
	ctl := f.controller(kube.KindDeployment, "arc-system", "gha-controller",
		0, 0, "app=gha")
	f.grant(f.accountOf(ctl), "actions.summerwind.dev "+arcGroup)
	stuck := f.runnerSet()

	c := requireCause(t, f.explain(), stuck[0], ctl.String())

	if c.Mode != ModeHandlerScaledDown {
		t.Fatalf("mode = %q, want %q", c.Mode, ModeHandlerScaledDown)
	}
}

// A role that grants the group to another ServiceAccount is no evidence.
func TestRoleOfAnotherAccountIsNotEvidence(t *testing.T) {
	f := newFixture(t)
	ctl := f.controller(kube.KindDeployment, "arc-system", "gha-controller",
		0, 0, "app=gha")
	f.accountOf(ctl)
	other := inventory.CoreID(kube.KindAccount, "arc-system", "someone")
	f.add(other)
	f.grant(other, arcGroup)
	stuck := f.runnerSet()

	notBlamed(t, f.explain(), stuck[0], ctl)
}

// The finalizer's own manager is the strongest proof, found by exact
// name in any namespace.
func TestFinalizerOwnerWorkloadIsBlamedWhereverItRuns(t *testing.T) {
	f := newFixture(t)
	f.controller(kube.KindDeployment, "ops", "widget-ctl", 0, 0, "app=a")
	owner := f.controller(kube.KindDeployment, "tools", "reaper", 0, 0, "")
	stuck := f.stuck("ops", 2, widgetFinalizer)
	for _, id := range stuck {
		f.set(id, map[string]inventory.Value{
			kube.AttrFinalizerManagers: inventory.Text(
				widgetFinalizer + "=reaper")})
	}

	requireCause(t, f.explain(), stuck[0], owner.String())
}

// One object whose finalizer a workload owns is enough: it is the
// handler of the finalizer for all the objects it holds.
func TestFinalizerOwnerOfOneObjectHandlesTheGroup(t *testing.T) {
	f := newFixture(t)
	owner := f.controller(kube.KindDeployment, "ops", "reaper", 0, 0, "")
	stuck := f.stuck("ops", 3, widgetFinalizer)
	f.set(stuck[1], map[string]inventory.Value{
		kube.AttrFinalizerManagers: inventory.Text(
			widgetFinalizer + "=reaper")})

	e := f.explain()

	for _, id := range stuck {
		requireCause(t, e, id, owner.String())
	}
}

// A manager that matches no workload by exact name proves nothing.
func TestFinalizerOwnerWithoutAWorkloadIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	ctl := f.controller(kube.KindDeployment, "ops", "reaper-2", 0, 0, "")
	stuck := f.stuck("ops", 2, widgetFinalizer)
	f.set(stuck[0], map[string]inventory.Value{
		kube.AttrFinalizerManagers: inventory.Text(
			widgetFinalizer + "=reaper")})

	notBlamed(t, f.explain(), stuck[0], ctl)
}

// The workload that last wrote the object is not the one that removes
// its finalizer: a spec writer proves nothing.
func TestSpecWriterIsNotProofOfAFinalizerHandler(t *testing.T) {
	f := newFixture(t)
	ctl := f.controller(kube.KindDeployment, "ops", "legacy", 0, 0, "")
	stuck := f.stuck("ops", 3, widgetFinalizer)
	for _, id := range stuck {
		f.links[id] = []Link{{Type: inventory.ManagedBy, To: ctl}}
	}

	c := notBlamed(t, f.explain(), stuck[0], ctl)

	if c.Mode != ModeFinalizerUnhandled {
		t.Fatalf("mode = %q", c.Mode)
	}
}

// Pod annotations in the group's domain tie a workload to the group.
func TestAnnotatedControllerIsBlamed(t *testing.T) {
	f := newFixture(t)
	ctl := f.controller(kube.KindDeployment, "tools", "any-name", 0, 0, "")
	f.set(ctl, map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(0),
		kube.AttrReadyReplicas: inventory.Number(0),
		kube.AttrTemplateAnnotationDomains: inventory.Text(
			"widgets.example.com")})
	stuck := f.stuck("ops", 2, widgetFinalizer)

	requireCause(t, f.explain(), stuck[0], ctl.String())
}

// Arguments that quote the group or the finalizer tie a workload to it.
func TestControllerQuotingTheGroupIsBlamed(t *testing.T) {
	for _, quoted := range []string{"widgets.example.com", widgetFinalizer} {
		f := newFixture(t)
		ctl := f.controller(kube.KindStatefulSet, "tools", "any-name", 2, 0,
			"")
		f.set(ctl, map[string]inventory.Value{
			kube.AttrReplicas:      inventory.Number(2),
			kube.AttrReadyReplicas: inventory.Number(0),
			kube.AttrTemplateQuoted: inventory.Text("other.example.com " +
				quoted)})
		stuck := f.stuck("ops", 2, widgetFinalizer)

		c := requireCause(t, f.explain(), stuck[0], ctl.String())

		if c.Mode != ModeHandlerNotReady {
			t.Fatalf("%s: mode = %q", quoted, c.Mode)
		}
	}
}

// The controller of a group lives wherever its operator was installed,
// not in the namespace of the objects it holds.
func TestLabelledControllerOfAnotherNamespaceIsBlamed(t *testing.T) {
	f := newFixture(t)
	ctl := f.controller(kube.KindDeployment, "widget-system", "widget-ctl",
		0, 0, "widgets.example.com/controller-namespace=ops")
	stuck := f.stuck("ops", 2, widgetFinalizer)

	requireCause(t, f.explain(), stuck[0], ctl.String())
}

// Cluster-scoped objects have no namespace; their controller is found
// cluster-wide all the same.
func TestControllerOfClusterScopedObjectsIsFoundClusterWide(t *testing.T) {
	f := newFixture(t)
	ctl := f.controller(kube.KindDeployment, "kube-system", "widget-ctl", 0,
		0, "widgets.example.com/controller=x")
	stuck := f.stuckOf("", "volumeattachment", "", 2, widgetFinalizer)

	requireCause(t, f.explain(), stuck[0], ctl.String())
}

// A webhook that admits the group is served by the group's controller.
func TestControllerBehindTheGroupsWebhookIsBlamed(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("tools", "hook-server", 1)
	ctl := inventory.CoreID(kube.KindDeployment, "tools", "hook-server")
	f.set(ctl, map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(1),
		kube.AttrReadyReplicas: inventory.Number(0)})
	service := inventory.CoreID(kube.KindService, "tools", "hook")
	f.add(service)
	f.links[service] = []Link{{Type: inventory.Selects, To: pods[0]}}
	hook := inventory.CoreID(kube.KindValidatingHook, "", "widgets")
	f.set(hook, map[string]inventory.Value{
		kube.AttrWebhookGroups: inventory.Text("widgets.example.com")})
	f.relate(hook, inventory.Serves, service)
	stuck := f.stuck("ops", 2, widgetFinalizer)

	c := requireCause(t, f.explain(), stuck[0], ctl.String())

	if c.Mode != ModeHandlerNotReady {
		t.Fatalf("mode = %q", c.Mode)
	}
}

// A CRD of the group whose conversion webhook a workload serves does
// the same.
func TestControllerBehindTheGroupsConversionWebhookIsBlamed(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("tools", "hook-server", 1)
	ctl := inventory.CoreID(kube.KindDeployment, "tools", "hook-server")
	f.set(ctl, map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(1),
		kube.AttrReadyReplicas: inventory.Number(0)})
	service := inventory.CoreID(kube.KindService, "tools", "convert")
	f.add(service)
	f.links[service] = []Link{{Type: inventory.Selects, To: pods[0]}}
	crd := inventory.CoreID(kube.KindCRD, "", "widgets.widgets.example.com")
	f.add(crd)
	f.relate(crd, inventory.Serves, service)
	stuck := f.stuck("ops", 2, widgetFinalizer)

	requireCause(t, f.explain(), stuck[0], ctl.String())
}

// With positive evidence for a controller that runs, the finalizer is
// blamed and its controller is not.
func TestRunningEvidencedControllerIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	ctl := f.controller(kube.KindDeployment, "arc-system", "gha-controller",
		1, 1, "app=gha")
	f.grant(f.accountOf(ctl), arcGroup)
	stuck := f.runnerSet()

	c := notBlamed(t, f.explain(), stuck[0], ctl)

	if c.Mode != ModeFinalizerHandlerRuns {
		t.Fatalf("mode = %q, want %q", c.Mode, ModeFinalizerHandlerRuns)
	}
}

// The owner of the finalizer is usually the workload that also wrote the
// objects: a walk reaches it by the manages link, and the same evidence
// still decides.
func TestFinalizerOwnerThatIsAlsoTheSpecWriterIsBlamed(t *testing.T) {
	f := newFixture(t)
	owner := f.controller(kube.KindDeployment, "tools", "reaper", 0, 0, "")
	stuck := f.stuck("ops", 4, widgetFinalizer)
	for _, id := range stuck {
		f.set(id, map[string]inventory.Value{
			kube.AttrFinalizerManagers: inventory.Text(
				widgetFinalizer + "=reaper")})
		f.links[id] = []Link{{Type: inventory.ManagedBy, To: owner}}
	}

	c := requireCause(t, f.explain(), stuck[0], owner.String())

	if c.Row != "finalizer-handler-stopped" {
		t.Fatalf("row = %q", c.Row)
	}
}
