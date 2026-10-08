package explain

import (
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func init() {
	pseudoModes = append(pseudoModes, ModeHandlerStopped,
		ModeFinalizerUnhandled)
}

const widgetFinalizer = "widgets.example.com/cleanup"

// controller adds a workload of ns with its replica counts and the
// labels of its pods.
func (f *fixture) controller(
	kind inventory.Kind, ns, name string, want, ready float64,
	labels string,
) inventory.EntityID {
	id := inventory.CoreID(kind, ns, name)
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrReplicas:       inventory.Number(want),
			kube.AttrReadyReplicas:  inventory.Number(ready),
			kube.AttrTemplateLabels: inventory.Text(labels),
		}})
	return id
}

// stuck adds n Secrets of ns whose deletion the finalizers hold.
func (f *fixture) stuck(
	ns string, n int, finalizers ...string,
) []inventory.EntityID {
	var out []inventory.EntityID
	for i := 0; i < n; i++ {
		id := inventory.CoreID(kube.KindSecret, ns,
			"old-"+path.Base(finalizers[0])+"-"+strconv.Itoa(i))
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

var finalizerRowCases = []rowCase{
	{row: "finalizer-handler-stopped", want: "deployment/ops/widget-ctl",
		build: func(f *fixture) inventory.EntityID {
			f.controller(kube.KindDeployment, "ops", "widget-ctl", 0, 0,
				"widgets.example.com/controller-namespace=ops")
			return f.stuck("ops", 3, widgetFinalizer)[0]
		}},
	{row: "finalizer-unhandled", want: "finalizer/ops/" + widgetFinalizer,
		build: func(f *fixture) inventory.EntityID {
			return f.stuck("ops", 2, widgetFinalizer)[0]
		}},
}

// rootOf solves the fixture and names the cause of effect.
func rootOf(f *fixture, effect inventory.EntityID) (Cause, bool) {
	return f.explain().CauseOf(effect)
}

// A controller that runs is not blamed: the finalizer is, and the cause
// says the controller was seen running.
func TestRunningFinalizerControllerIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	f.controller(kube.KindDeployment, "ops", "widget-ctl", 1, 1,
		"widgets.example.com/controller-namespace=ops")
	effect := f.stuck("ops", 2, widgetFinalizer)[0]

	c, ok := rootOf(f, effect)

	if !ok || c.Root.Kind != KindFinalizer ||
		c.Mode != ModeFinalizerHandlerRuns {
		t.Fatalf("cause = %v mode %q, want the finalizer, handler running",
			c.Root, c.Mode)
	}
}

// A controller that wants replicas and has none ready is a stopped
// controller too, and the mode says so.
func TestNotReadyFinalizerControllerIsBlamed(t *testing.T) {
	f := newFixture(t)
	f.controller(kube.KindStatefulSet, "ops", "widget-ctl", 2, 0,
		"widgets.example.com/controller-namespace=ops")
	effect := f.stuck("ops", 1, widgetFinalizer)[0]

	c := requireCause(t, f.explain(), effect, "statefulset/ops/widget-ctl")

	if c.Mode != ModeHandlerNotReady {
		t.Fatalf("mode = %q, want %q", c.Mode, ModeHandlerNotReady)
	}
}

// Only concrete evidence names a controller: a scaled-down workload of
// the namespace that has nothing to do with the finalizer is left out,
// and so is one of another namespace with the right labels.
func TestUnrelatedStoppedWorkloadIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	f.controller(kube.KindDeployment, "ops", "batch-worker", 0, 0,
		"app=batch-worker")
	f.controller(kube.KindDeployment, "other", "widget-ctl", 0, 0,
		"widgets.example.com/controller-namespace=other")
	effect := f.stuck("ops", 2, widgetFinalizer)[0]

	c, ok := rootOf(f, effect)

	if !ok || c.Root.Kind != KindFinalizer ||
		c.Mode != ModeFinalizerUnhandled {
		t.Fatalf("cause = %v mode %q, want the finalizer alone", c.Root,
			c.Mode)
	}
}

// A workload named after the finalizer's group is the weakest evidence;
// it counts when nothing better is found.
func TestControllerNamedAfterTheGroupIsBlamed(t *testing.T) {
	f := newFixture(t)
	f.controller(kube.KindDeployment, "ops", "widgets-controller", 0, 0,
		"app=widgets-controller")
	effect := f.stuck("ops", 2, widgetFinalizer)[0]

	requireCause(t, f.explain(), effect, "deployment/ops/widgets-controller")
}

// A workload that wrote the object is its controller, whatever it is
// named or labelled.
func TestFieldManagerNamesTheFinalizerController(t *testing.T) {
	f := newFixture(t)
	ctl := f.controller(kube.KindDeployment, "ops", "reaper", 0, 0, "")
	effect := f.stuck("ops", 1, widgetFinalizer)[0]
	f.links[effect] = []Link{{Type: inventory.ManagedBy, To: ctl}}

	requireCause(t, f.explain(), effect, "deployment/ops/reaper")
}

// Objects held by Kubernetes' own finalizers, and one object held by a
// finalizer nobody else shares, are not grouped.
func TestBuiltinAndSingleFinalizersMakeNoGroup(t *testing.T) {
	f := newFixture(t)
	claims := f.stuck("ops", 2, "kubernetes.io/pvc-protection")
	single := f.stuck("ops2", 1, widgetFinalizer)[0]

	e := f.explain()

	for _, id := range append(claims, single) {
		if c, ok := e.CauseOf(id); ok && c.Root.Kind == KindFinalizer {
			t.Errorf("%s is grouped under %s", id, c.Root)
		}
	}
}

// Different finalizers are different leftovers.
func TestDifferentFinalizersAreSeparateCauses(t *testing.T) {
	f := newFixture(t)
	a := f.stuck("ops", 2, "widgets.example.com/a")
	b := f.stuck("ops", 2, "gadgets.example.com/b")

	e := f.explain()

	for id, want := range map[inventory.EntityID]string{
		a[0]: "widgets.example.com/a", b[0]: "gadgets.example.com/b"} {
		if c, ok := e.CauseOf(id); !ok || c.Root.Name != want {
			t.Errorf("cause of %s = %v, want finalizer %s", id, c.Root,
				want)
		}
	}
}
