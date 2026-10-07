package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// configChangeMinute is when the fixture's ConfigMap changes.
const configChangeMinute = 3

// startedPod gives a pod a creation time, minutes after t0, and the
// frozen reference that makes it read config. A healthy pod is ready;
// an observation replaces a pod's attributes, so it is set here too.
func (f *fixture) startedPod(
	pod inventory.EntityID, minutes int, frozen string, healthy bool,
) {
	at := t0.Add(time.Duration(minutes) * time.Minute)
	attrs := map[string]inventory.Value{
		kube.AttrCreated:           inventory.Time(at),
		kube.AttrContainersStarted: inventory.Time(at),
	}
	if healthy {
		attrs[kube.AttrReady] = inventory.Bool(true)
		attrs[kube.AttrReadySince] = inventory.Time(t0.Add(-time.Hour))
	}
	if frozen != "" {
		attrs[kube.AttrFrozenConfig] = inventory.Text(frozen)
	}
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: pod,
		Attributes: attrs})
}

// configSplit is a workload of three old pods (started an hour before
// the change) and two new ones, all using api-config through frozen.
// The old ones are healthy and the new ones crash, or the reverse.
func configSplit(
	t *testing.T, kind inventory.Kind, frozen string, newFail bool,
) (*fixture, inventory.EntityID) {
	f := newFixture(t)
	cm := inventory.CoreID(kind, "shop", "api-config")
	f.add(cm)
	pods := f.workload("shop", "api", 5, f.nodes("z", "n1", "n2")...)
	for i, pod := range pods {
		isOld := i < 3
		f.relate(pod, inventory.References, cm)
		minutes := configChangeMinute + 2
		if isOld {
			minutes = -60
		}
		healthy := isOld == newFail
		f.startedPod(pod, minutes, frozen, healthy)
		if !healthy {
			f.crash(pod)
		}
	}
	f.change(cm, configChangeMinute, "data.DB_HOST", "data.TIMEOUT")
	return f, cm
}

func TestConfigVersionNewPodsFail(t *testing.T) {
	f, cm := configSplit(t, kube.KindConfigMap,
		"configmap/api-config", true)
	o := scoreOf(t, f, cm, scoreConfigVersion)
	requireWeight(t, o, ConfigVersionWeight)
	if o.code != rootcause.ProofNewConfigFails || o.count != 2 ||
		o.total != 3 {
		t.Fatalf("code=%q count=%d total=%d", o.code, o.count, o.total)
	}
}

func TestConfigVersionOldPodsFail(t *testing.T) {
	f, cm := configSplit(t, kube.KindConfigMap,
		"configmap/api-config", false)
	o := scoreOf(t, f, cm, scoreConfigVersion)
	requireWeight(t, o, ConfigVersionWeight)
	if o.code != rootcause.ProofOldConfigFails || o.count != 3 ||
		o.total != 2 {
		t.Fatalf("code=%q count=%d total=%d", o.code, o.count, o.total)
	}
}

func TestConfigVersionWorksForSecrets(t *testing.T) {
	f, cm := configSplit(t, kube.KindSecret, "secret/api-config", true)
	requireWeight(t, scoreOf(t, f, cm, scoreConfigVersion),
		ConfigVersionWeight)
}

// A plain volume mount refreshes in place: no pod runs stale content.
func TestConfigVersionSkipsPlainVolumes(t *testing.T) {
	f, cm := configSplit(t, kube.KindConfigMap, "", true)
	requireWeight(t, scoreOf(t, f, cm, scoreConfigVersion), 0)
}

func TestConfigVersionNeedsTheSplit(t *testing.T) {
	f, cm := configSplit(t, kube.KindConfigMap,
		"configmap/api-config", true)
	for _, pod := range rootcause.OwnedPods(f.model,
		deployment("shop", "api")) {
		f.crash(pod)
	}
	requireWeight(t, scoreOf(t, f, cm, scoreConfigVersion), 0)
}

// A pod that restarted after the change may run either content.
func TestConfigVersionIgnoresRestartedPods(t *testing.T) {
	f, cm := configSplit(t, kube.KindConfigMap,
		"configmap/api-config", true)
	pods := rootcause.OwnedPods(f.model, deployment("shop", "api"))
	for _, pod := range pods {
		e, _ := f.model.Entity(pod)
		if !e.Attributes[kube.AttrCreated].Value.AsTime().Before(t0) {
			continue
		}
		attrs := map[string]inventory.Value{}
		for name, attr := range e.Attributes {
			attrs[name] = attr.Value
		}
		attrs[kube.AttrContainersStarted] = inventory.Time(
			t0.Add(time.Hour))
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: pod, Attributes: attrs})
	}
	requireWeight(t, scoreOf(t, f, cm, scoreConfigVersion), 0)
}
