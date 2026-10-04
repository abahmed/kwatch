package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const (
	failingH  = detection.Failing
	degradedH = detection.Degraded
)

// rowCase is one propagation row as a fixture: a graph in, the blamed
// root and the row that linked it out.
type rowCase struct {
	row   string
	build func(f *fixture) inventory.EntityID
	want  string
}

// nodeCase builds pods of two workloads on node n1, failing in
// effectMode while n1 is in nodeMode and a replica on n2 is healthy.
func nodeCase(
	nodeMode, effectMode detection.Mode,
) func(*fixture) inventory.EntityID {
	return func(f *fixture) inventory.EntityID {
		nodes := f.nodes("zone-a", "n1", "n2")
		f.fail(nodes[0], nodeMode, failingH, 1, "")
		a := f.workload("shop", "api", 2, nodes...)
		b := f.workload("shop", "web", 2, nodes...)
		f.fail(a[0], effectMode, failingH, 2, "")
		f.fail(b[0], effectMode, failingH, 2, "")
		return a[0]
	}
}

var genericRowCases = []rowCase{
	{row: "owner-failing", want: "postgrescluster.pg.example/data/db",
		build: func(f *fixture) inventory.EntityID {
			cr := inventory.NewEntityID("pg.example", "postgrescluster",
				"data", "db")
			pod := inventory.CoreID(kube.KindPod, "data", "db-0")
			f.add(cr, pod)
			f.relate(pod, inventory.OwnedBy, cr)
			f.fail(cr, "Failed", failingH, 1, "")
			f.fail(pod, "CrashLoop", failingH, 2, "")
			return pod
		}},
	{row: "used-missing", want: "serviceaccount/shop/robot",
		build: func(f *fixture) inventory.EntityID {
			pods := f.workload("shop", "api", 2)
			sa := inventory.CoreID(kube.KindAccount, "shop", "robot")
			for _, pod := range pods {
				f.relate(pod, inventory.References, sa)
				f.fail(pod, "Missing.ServiceAccount", failingH, 2, "")
			}
			return pods[0]
		}},
	{row: "controller-failing", want: "deployment/ops/pg-operator",
		build: func(f *fixture) inventory.EntityID {
			op := inventory.CoreID(kube.KindDeployment, "ops", "pg-operator")
			cr := inventory.NewEntityID("pg.example", "postgrescluster",
				"data", "db")
			f.add(op, cr)
			f.links[cr] = []Link{{Type: inventory.ManagedBy, To: op}}
			f.fail(op, "ReplicaFailure", failingH, 1, "")
			f.fail(cr, "NotReconciling", degradedH, 2, "")
			return cr
		}},
	{row: "service-no-endpoints", want: "service/policy/hook",
		build: func(f *fixture) inventory.EntityID {
			svc := inventory.CoreID(kube.KindService, "policy", "hook")
			api := inventory.CoreID(kube.KindAPIService, "", "v1.x.example")
			f.add(svc, api)
			f.relate(api, inventory.Serves, svc)
			f.fail(svc, "NoEndpoints", failingH, 1, "")
			f.fail(api, "APIServiceUnavailable", degradedH, 2, "")
			return api
		}},
	{row: "apiservice-unavailable", want: "apiservice//v1.x.example",
		build: func(f *fixture) inventory.EntityID {
			api := inventory.CoreID(kube.KindAPIService, "", "v1.x.example")
			cr := inventory.NewEntityID("x.example", "widget", "shop", "w")
			f.add(api, cr)
			f.relate(cr, inventory.Serves, api)
			f.fail(api, "APIServiceUnavailable", degradedH, 1, "")
			f.fail(cr, "NotReconciling", degradedH, 2, "")
			return cr
		}},
	{row: "namespace-terminating", want: "namespace//old",
		build: func(f *fixture) inventory.EntityID {
			ns := inventory.CoreID(kube.KindNamespace, "", "old")
			cm := inventory.CoreID(kube.KindConfigMap, "old", "c")
			f.add(ns, cm)
			f.fail(ns, "StuckDeleting", failingH, 1, "")
			f.fail(cm, "StuckDeleting", degradedH, 2, "")
			return cm
		}},
	{row: "backends-failing", want: "pod/shop/solo",
		build: func(f *fixture) inventory.EntityID {
			pod := inventory.CoreID(kube.KindPod, "shop", "solo")
			svc := inventory.CoreID(kube.KindService, "shop", "solo")
			f.add(pod, svc)
			f.links[svc] = []Link{{Type: inventory.Selects, To: pod}}
			f.fail(pod, "CrashLoop", failingH, 1, "")
			f.fail(svc, "NoEndpoints", failingH, 2, "")
			return svc
		}},
	{row: "own-change", want: "httproute.gateway.networking.k8s.io/shop/web",
		build: func(f *fixture) inventory.EntityID {
			route := inventory.NewEntityID("gateway.networking.k8s.io",
				"httproute", "shop", "web")
			f.add(route)
			f.change(route, 1, "spec")
			f.fail(route, "Condition.Accepted", degradedH, 2, "")
			return route
		}},
	{row: "apiserver-unavailable", want: "apiserver//kube-apiserver",
		build: func(f *fixture) inventory.EntityID {
			f.fail(kube.APIServer, "Unavailable.APIServer", failingH, 1, "")
			for _, name := range []string{"a", "b"} {
				cr := inventory.NewEntityID("x.example", "widget", "shop",
					name)
				f.add(cr)
				f.fail(cr, "NotReconciling", failingH, 2, "")
			}
			return inventory.NewEntityID("x.example", "widget", "shop", "a")
		}},
}

var nodeRowCases = []rowCase{
	{row: "node-not-ready", want: "node//n1",
		build: nodeCase("NotReady", "NotReady")},
	{row: "node-memory-pressure", want: "node//n1",
		build: nodeCase("MemoryPressure", "Evicted")},
	{row: "node-memory-pressure-unready", want: "node//n1",
		build: nodeCase("MemoryPressure", "NotReady")},
	{row: "node-disk-pressure", want: "node//n1",
		build: nodeCase("DiskPressure", "Evicted")},
	{row: "node-pid-pressure", want: "node//n1",
		build: nodeCase("PIDPressure", "CannotRun")},
	{row: "node-network", want: "node//n1",
		build: nodeCase("NetworkUnavailable", "Creating")},
	{row: "node-overcommitted", want: "node//n1",
		build: nodeCase("MemoryOvercommitted", "OOMKilled")},
}
