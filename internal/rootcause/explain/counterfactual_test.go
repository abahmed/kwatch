package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// ready marks pods as up and ready since an hour before t0.
func (f *fixture) ready(pods ...inventory.EntityID) {
	for _, pod := range pods {
		f.apply(inventory.Observation{Kind: inventory.Observed, Entity: pod,
			Attributes: map[string]inventory.Value{
				kube.AttrReady: inventory.Bool(true),
				kube.AttrReadySince: inventory.Time(
					t0.Add(-time.Hour)),
			}})
	}
}

// pullImage makes the pods pull image.
func (f *fixture) pullImage(image string, pods ...inventory.EntityID) {
	id := inventory.CoreID(kube.KindImage, "", image)
	f.add(id)
	for _, pod := range pods {
		f.relate(pod, inventory.Pulls, id)
		f.relate(containerOf(pod), inventory.Pulls, id)
	}
}

// crash makes every pod crash-loop from minute 2.
func (f *fixture) crash(pods ...inventory.EntityID) {
	for _, pod := range pods {
		f.fail(pod, detection.ModeCrashLoop, failingH, 2, "panic")
	}
}

func deployment(namespace, name string) inventory.EntityID {
	return inventory.CoreID(kube.KindDeployment, namespace, name)
}

// imageFixture is a Deployment whose release changed the image of its
// two crashing pods, and the same image running in twin namespace.
func imageFixture(t *testing.T, twinHealthy bool) *fixture {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	pods := f.workload("shop", "api", 2, nodes...)
	f.pullImage("api:2.3", pods...)
	f.crash(pods...)
	f.change(deployment("shop", "api"), 1,
		"spec.template.spec.containers[0].image")
	twin := f.workload("staging-2", "api", 2, nodes...)
	f.pullImage("api:2.3", twin...)
	if twinHealthy {
		f.ready(twin...)
	} else {
		f.crash(twin...)
	}
	return f
}

func TestScoreImageElsewhere(t *testing.T) {
	dep := deployment("shop", "api")
	t.Run("healthy twin lowers an image cause", func(t *testing.T) {
		f := imageFixture(t, true)
		o := scoreOf(t, f, dep, scoreImageElsewhere)
		requireWeight(t, o, -ImageElsewhereWeight)
		if o.code != rootcause.ProofImageElsewhere {
			t.Fatalf("code = %q", o.code)
		}
	})
	t.Run("failing twin says nothing", func(t *testing.T) {
		f := imageFixture(t, false)
		requireWeight(t, scoreOf(t, f, dep, scoreImageElsewhere), 0)
	})
	t.Run("no twin says nothing", func(t *testing.T) {
		f := newFixture(t)
		pods := f.workload("shop", "api", 2)
		f.pullImage("api:2.3", pods...)
		f.crash(pods...)
		f.change(dep, 1, "spec.template.spec.containers[0].image")
		requireWeight(t, scoreOf(t, f, dep, scoreImageElsewhere), 0)
	})
	t.Run("recorded in the trace", func(t *testing.T) {
		e := imageFixture(t, true).explain()
		for _, area := range e.Areas {
			for effect, list := range area.Trace.Compared {
				if len(list) > 0 &&
					list[0].Code == rootcause.ProofImageElsewhere {
					t.Logf("%s: %s", effect, list[0].Text)
					return
				}
			}
		}
		t.Fatal("no image comparison in the trace")
	})
}

func TestScoreImageElsewhereRaisesSurroundings(t *testing.T) {
	f := imageFixture(t, true)
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "api-config")
	f.add(cm)
	for _, pod := range rootcause.OwnedPods(f.model,
		deployment("shop", "api")) {
		f.relate(pod, inventory.References, cm)
	}
	f.change(cm, 1, "data.app.yaml")
	requireWeight(t, scoreOf(t, f, cm, scoreImageElsewhere),
		ImageElsewhereWeight)
}

// revision sets a ReplicaSet's Deployment revision.
func (f *fixture) revision(rs inventory.EntityID, revision string) {
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: rs,
		Attributes: map[string]inventory.Value{
			kube.AttrRevision: inventory.Text(revision)}})
}

// readyFor marks a pod ready since minutes before the fixture's change
// at minute 20 (see TestScorePreviousHealthy).
func (f *fixture) readyFor(pod inventory.EntityID, minutes int) {
	since := t0.Add(20*time.Minute - time.Duration(minutes)*time.Minute)
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: pod,
		Attributes: map[string]inventory.Value{
			kube.AttrReady:      inventory.Bool(true),
			kube.AttrReadySince: inventory.Time(since)}})
}
