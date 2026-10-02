package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var controlPlaneRowCases = []rowCase{
	{row: "scheduler-unavailable", want: "scheduler//kube-scheduler",
		build: func(f *fixture) inventory.EntityID {
			f.fail(kube.Scheduler, "Unavailable.Scheduler", failingH, 1, "")
			var first inventory.EntityID
			for _, name := range []string{"api", "web"} {
				pod := f.workload("shop", name, 1)[0]
				f.fail(pod, "Pending", degradedH, 2, "")
				if first.IsZero() {
					first = pod
				}
			}
			return first
		}},
	{row: "controller-failing",
		want: "controller-manager//kube-controller-manager",
		build: func(f *fixture) inventory.EntityID {
			f.fail(kube.ControllerManager, "Unavailable.ControllerManager",
				failingH, 1, "")
			f.workload("shop", "api", 1)
			f.workload("shop", "web", 1)
			api := inventory.CoreID(kube.KindDeployment, "shop", "api")
			web := inventory.CoreID(kube.KindDeployment, "shop", "web")
			f.fail(api, "NotReconciling", degradedH, 2, "")
			f.fail(web, "NotReconciling", degradedH, 2, "")
			return api
		}},
	{row: "etcd-unavailable", want: "etcd//etcd",
		build: func(f *fixture) inventory.EntityID {
			f.fail(kube.Etcd, "Unavailable.Etcd", failingH, 1, "")
			f.fail(kube.APIServer, "Unavailable.APIServer", failingH, 2, "")
			return kube.APIServer
		}},
}
