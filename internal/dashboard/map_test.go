package dashboard

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// mapShop is a namespace "shop": an Ingress routing to Service web,
// which selects the pods of Deployment web; web calls Service db in
// "data", whose pods belong to StatefulSet db. Namespace "hidden" is
// out of scope.
func mapShop(t *testing.T) (*testModel, map[string]inventory.EntityID) {
	tm := newTestModel(t)
	ids := map[string]inventory.EntityID{
		"ing":     inventory.CoreID(kube.KindIngress, "shop", "web"),
		"svc":     inventory.CoreID(kube.KindService, "shop", "web"),
		"deploy":  inventory.CoreID(kube.KindDeployment, "shop", "web"),
		"rs":      inventory.CoreID(kube.KindReplicaSet, "shop", "web-7d9f"),
		"pod":     inventory.CoreID(kube.KindPod, "shop", "web-7d9f-a1"),
		"db":      inventory.CoreID(kube.KindService, "data", "db"),
		"sts":     inventory.CoreID(kube.KindStatefulSet, "data", "db"),
		"dbpod":   inventory.CoreID(kube.KindPod, "data", "db-0"),
		"node":    inventory.CoreID(kube.KindNode, "", "n1"),
		"hidden":  inventory.CoreID(kube.KindService, "hidden", "x"),
		"hiddenp": inventory.CoreID(kube.KindPod, "hidden", "x-0"),
	}
	for _, ns := range []string{"shop", "data", "hidden"} {
		tm.observe(inventory.CoreID(kube.KindNamespace, "", ns), nil)
	}
	tm.observe(ids["deploy"], map[string]inventory.Value{
		kube.AttrReplicas:      inventory.Number(3),
		kube.AttrReadyReplicas: inventory.Number(2),
	})
	for _, k := range []string{"ing", "svc", "rs", "pod", "db", "sts",
		"dbpod", "node", "hidden", "hiddenp"} {
		tm.observe(ids[k], nil)
	}
	tm.relate(ids["pod"], inventory.OwnedBy, ids["rs"])
	tm.relate(ids["rs"], inventory.OwnedBy, ids["deploy"])
	tm.relate(ids["dbpod"], inventory.OwnedBy, ids["sts"])
	tm.relate(ids["ing"], inventory.RoutesTo, ids["svc"])
	tm.relate(ids["svc"], inventory.Selects, ids["pod"])
	tm.relate(ids["db"], inventory.Selects, ids["dbpod"])
	tm.relate(ids["pod"], inventory.Calls, ids["db"], ids["hidden"])
	tm.relate(ids["pod"], inventory.RunsOn, ids["node"])
	return tm, ids
}

func edgeSet(out *IssueMap) map[MapEdge]bool {
	set := map[MapEdge]bool{}
	for _, e := range out.Edges {
		set[e] = true
	}
	return set
}

func nodeByID(out *IssueMap, id inventory.EntityID) (MapNode, bool) {
	for _, n := range out.Nodes {
		if n.ID == nodeID(id) {
			return n, true
		}
	}
	return MapNode{}, false
}

var mapTestScope = Scope{Excluded: []string{"hidden"}}

// An issue's picture shows its workload (pods folded in) and what is
// next to it: the Service and the route in front, the Service it calls
// in another namespace. Out-of-scope namespaces never appear.
func TestIssueMapShowsWhatTheIssueTouchesAndItsNeighbours(t *testing.T) {
	tm, ids := mapShop(t)
	key := detection.Key{Entity: ids["pod"], Reason: "CrashLoopBackOff"}
	p := incident.Incident{ID: "inc-web", Root: ids["deploy"],
		Tier: incident.Page, State: incident.Open,
		Members: map[detection.Key]detection.Finding{key: {}}}
	out := issueMap(tm.m, mapTestScope, []incident.Incident{p}, p)
	if out == nil {
		t.Fatal("no picture")
	}
	if out.Self != nodeID(ids["deploy"]) {
		t.Errorf("self = %q, want the deployment", out.Self)
	}
	edges := edgeSet(out)
	for _, want := range []MapEdge{
		{From: nodeID(ids["svc"]), To: nodeID(ids["deploy"]),
			Type: "selects"},
		{From: nodeID(ids["deploy"]), To: nodeID(ids["db"]),
			Type: "calls"},
	} {
		if !edges[want] {
			t.Errorf("missing edge %+v in %+v", want, out.Edges)
		}
	}
	web, _ := nodeByID(out, ids["deploy"])
	if web.Level != "page" || web.Issue != "inc-web" ||
		web.Ready != "2/3" {
		t.Errorf("deployment node %+v", web)
	}
	if db, ok := nodeByID(out, ids["db"]); !ok || !db.External {
		t.Errorf("db: %+v %v, want an external neighbour", db, ok)
	}
	if _, ok := nodeByID(out, ids["hidden"]); ok {
		t.Error("an out-of-scope namespace leaked into the picture")
	}
	if _, ok := nodeByID(out, ids["pod"]); ok {
		t.Error("the pod was drawn instead of folded into its workload")
	}
}

// An object kwatch knows no connection of gets no picture.
func TestIssueMapWithoutConnectionsIsNil(t *testing.T) {
	tm, ids := mapShop(t)
	p := incident.Incident{ID: "inc-node", Root: ids["node"],
		Tier: incident.Notify, State: incident.Open}
	if out := issueMap(tm.m, mapTestScope, nil, p); out != nil {
		t.Errorf("picture for an unconnected object: %+v", out)
	}
}
