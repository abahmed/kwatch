package dashboard

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// reasonsFixture is a Deployment whose pod fails, with the objects
// around it each linked the way the model links them.
func reasonsFixture(
	t *testing.T,
) (*testModel, incident.Incident, map[string]inventory.EntityID) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	rs := inventory.CoreID(kube.KindReplicaSet, "shop", "api-7f")
	tm.observe(rs, nil)
	tm.relate(rs, inventory.OwnedBy, root)
	tm.relate(pod, inventory.OwnedBy, rs)
	ids := map[string]inventory.EntityID{
		"deployment": root, "pod": pod, "replicaset": rs,
		"mounted":  inventory.CoreID(kube.KindConfigMap, "shop", "cfg"),
		"account":  inventory.CoreID(kube.KindAccount, "shop", "api"),
		"service":  inventory.CoreID(kube.KindService, "shop", "api"),
		"node":     inventory.CoreID(kube.KindNode, "", "n1"),
		"policy":   inventory.CoreID(kube.KindNetworkPolicy, "shop", "deny"),
		"hpa":      inventory.CoreID(kube.KindHPA, "shop", "api"),
		"ingress":  inventory.CoreID(kube.KindIngress, "shop", "web"),
		"cause":    inventory.CoreID(kube.KindSecret, "shop", "tls"),
		"stranger": inventory.CoreID(kube.KindConfigMap, "shop", "x"),
	}
	for _, id := range ids {
		tm.observe(id, nil)
	}
	tm.relate(pod, inventory.Mounts, ids["mounted"])
	tm.relate(pod, inventory.References, ids["account"])
	tm.relate(pod, inventory.RunsOn, ids["node"])
	tm.relate(ids["service"], inventory.Selects, pod)
	tm.relate(ids["policy"], inventory.Selects, pod)
	tm.relate(ids["hpa"], inventory.Scales, root)
	tm.relate(ids["ingress"], inventory.RoutesTo, ids["service"])
	f := detection.Finding{Entity: pod, Reason: "CrashLoopBackOff",
		Since: testEpoch}
	p := incident.Incident{ID: "r", Root: root, State: incident.Open,
		Members: map[detection.Key]detection.Finding{f.Key(): f},
		Cause:   &rootcause.CauseRecord{Root: ids["cause"]}}
	return tm, p, ids
}

func TestChangeWhyNamesTheLinkToTheFailingPods(t *testing.T) {
	tm, p, ids := reasonsFixture(t)
	links := newChangeLinks(tm.m, p)
	image := imageChange(testEpoch, "api:1", "api:2")
	scale := inventory.Change{Fields: []inventory.FieldChange{
		{Path: "spec.replicas", Before: "2", After: "3"}}}
	for _, tc := range []struct {
		object string
		change inventory.Change
		want   string
	}{
		{"deployment", image, "This is the workload's own template."},
		{"deployment", scale,
			"This is the workload the failing pods belong to."},
		{"pod", scale, "This is one of the failing pods."},
		{"mounted", scale, "The failing pods mount this ConfigMap."},
		{"account", scale, "The failing pods use this ServiceAccount."},
		{"service", scale, "This Service routes to the failing pods."},
		{"node", scale, "This is the node the failing pods run on."},
		{"policy", scale, "This NetworkPolicy selects the failing pods."},
		{"hpa", scale, "This autoscaler scales the failing workload."},
		{"ingress", scale, "This Ingress sends traffic to the failing " +
			"pods through Service api."},
		{"cause", scale, "This is the object the likely cause names."},
		{"stranger", scale, ""},
	} {
		change := tc.change
		change.Entity = ids[tc.object]
		if got := links.why(change); got != tc.want {
			t.Errorf("%s: why = %q, want %q", tc.object, got, tc.want)
		}
	}
}

func TestChangeWhyIsEmptyWithoutAModel(t *testing.T) {
	_, p, ids := reasonsFixture(t)
	change := inventory.Change{Entity: ids["mounted"]}
	if got := newChangeLinks(nil, p).why(change); got != "" {
		t.Errorf("why without a model = %q", got)
	}
}

func TestDetailChangesSayWhyEachChangeIsShown(t *testing.T) {
	tm, p := changesFixture(t)
	d := detailOf(t, contextServer(tm.m, true, p), "c")

	whys := map[string]string{}
	for _, c := range d.Changes {
		whys[c.Name+"/"+c.When] = c.Why
	}
	want := map[string]string{
		"api/before":        "This is the workload's own template.",
		"api/during":        "This is the workload's own template.",
		"app-config/before": "The failing pods use this ConfigMap.",
	}
	for key, w := range want {
		if whys[key] != w {
			t.Errorf("%s: why = %q, want %q (all %v)", key,
				whys[key], w, whys)
		}
	}
}
