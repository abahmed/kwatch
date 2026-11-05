package dashboard

import (
	"net/http"
	"strconv"
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

func buildTestMap(
	tm *testModel, items []incident.Incident, view, ns string,
	nodes bool,
) MapResponse {
	b := newMapBuilder(tm.m, Scope{Excluded: []string{"hidden"}}, nodes)
	b.markLevels(items)
	return b.build(view, ns)
}

func edgeSet(out MapResponse) map[MapEdge]bool {
	set := map[MapEdge]bool{}
	for _, e := range out.Edges {
		set[e] = true
	}
	return set
}

func nodeByID(out MapResponse, id inventory.EntityID) (MapNode, bool) {
	for _, n := range out.Nodes {
		if n.ID == nodeID(id) {
			return n, true
		}
	}
	return MapNode{}, false
}

// Relations become edges, pods fold into their workload, a called
// Service of another namespace is drawn at the edge, and an excluded
// namespace never shows.
func TestMapNamespaceFoldsPodsAndDrawsRelations(t *testing.T) {
	tm, ids := mapShop(t)
	out := buildTestMap(tm, nil, mapViewNamespace, "shop", false)
	edges := edgeSet(out)
	for _, e := range []MapEdge{
		{nodeID(ids["ing"]), nodeID(ids["svc"]), "routes"},
		{nodeID(ids["svc"]), nodeID(ids["deploy"]), "selects"},
		{nodeID(ids["deploy"]), nodeID(ids["db"]), "calls"},
	} {
		if !edges[e] {
			t.Errorf("missing edge %+v in %+v", e, out.Edges)
		}
	}
	if _, ok := nodeByID(out, ids["pod"]); ok {
		t.Error("pod drawn; it belongs in its Deployment")
	}
	if _, ok := nodeByID(out, ids["hidden"]); ok {
		t.Error("an excluded namespace's Service is drawn")
	}
	web, _ := nodeByID(out, ids["deploy"])
	if web.Ready != "2/3" || web.Level != mapLevelOK {
		t.Errorf("deployment node: %+v", web)
	}
	if db, ok := nodeByID(out, ids["db"]); !ok || !db.External {
		t.Errorf("called Service of another namespace: %+v %v", db, ok)
	}
	for _, ns := range out.Namespaces {
		if ns == "hidden" {
			t.Error("excluded namespace offered in the picker")
		}
	}
	if _, ok := nodeByID(out, ids["node"]); ok {
		t.Error("nodes drawn without ?nodes=1")
	}
	withNodes := buildTestMap(tm, nil, mapViewNamespace, "shop", true)
	if !edgeSet(withNodes)[MapEdge{nodeID(ids["deploy"]),
		nodeID(ids["node"]), "runs-on"}] {
		t.Error("runs-on edge missing with ?nodes=1")
	}
}

// An open issue colors the objects it touches, folded into their
// workload; the worst issue wins and a resolved one colors nothing.
func TestMapLevelsFollowOpenIssues(t *testing.T) {
	tm, ids := mapShop(t)
	pageKey := detection.Key{Entity: ids["pod"], Reason: "CrashLoopBackOff"}
	items := []incident.Incident{
		{ID: "inc-low", Root: ids["deploy"], Tier: incident.Digest,
			State: incident.Open},
		{ID: "inc-page", Root: ids["svc"], Tier: incident.Page,
			State:   incident.Open,
			Members: map[detection.Key]detection.Finding{pageKey: {}}},
		{ID: "inc-old", Root: ids["db"], Tier: incident.Page,
			State: incident.Resolved},
	}
	out := buildTestMap(tm, items, mapViewNamespace, "shop", false)
	web, _ := nodeByID(out, ids["deploy"])
	if web.Level != "page" || web.Issue != "inc-page" {
		t.Errorf("deployment: %+v, want the urgent issue", web)
	}
	if db, _ := nodeByID(out, ids["db"]); db.Level != mapLevelOK {
		t.Errorf("resolved issue colors db: %+v", db)
	}
	if def := buildTestMap(tm, items, mapViewNamespace, "", false); def.
		Namespace != "shop" {
		t.Errorf("default namespace %q, want the failing one",
			def.Namespace)
	}
	failing := buildTestMap(tm, items, mapViewFailing, "", false)
	if _, ok := nodeByID(failing, ids["ing"]); !ok {
		t.Errorf("failing view lacks the route next to the failing "+
			"Service: %+v", failing.Nodes)
	}
}

// Past maxMapNodes the answer draws nothing and says how many there are.
func TestMapTooManyNodes(t *testing.T) {
	tm := newTestModel(t)
	tm.observe(inventory.CoreID(kube.KindNamespace, "", "big"), nil)
	for i := range maxMapNodes + 1 {
		tm.observe(inventory.CoreID(kube.KindService, "big",
			"svc-"+strconv.Itoa(i)), nil)
	}
	out := buildTestMap(tm, nil, mapViewNamespace, "big", false)
	if !out.TooMany || len(out.Nodes) != 0 || out.Count != maxMapNodes+1 {
		t.Fatalf("too many: %v nodes %d count %d", out.TooMany,
			len(out.Nodes), out.Count)
	}
}

// The handler checks its parameters and answers 503 without a model.
func TestMapHandlerValidatesAndNeedsTheModel(t *testing.T) {
	s := newTestServer()
	for _, path := range []string{"/api/map?view=other",
		"/api/map?nodes=yes"} {
		if w := call(t, s, http.MethodGet, path, testToken); w.Code != 400 {
			t.Errorf("%s: %d, want 400", path, w.Code)
		}
	}
	if w := call(t, s, http.MethodGet, "/api/map", testToken); w.Code !=
		http.StatusServiceUnavailable {
		t.Errorf("/api/map without a model: %d, want 503", w.Code)
	}
}
