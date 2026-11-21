package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var roleTokenRowCases = []rowCase{
	{row: "role-lacks-grant", want: "role/shop/operator-role",
		build: func(f *fixture) inventory.EntityID {
			pod := f.workload("shop", "operator", 1)[0]
			f.add(inventory.CoreID(kube.KindRole, "shop", "operator-role"))
			f.fail(containerOf(pod), detection.ModeCrashLoop, failingH, 2,
				"leases is forbidden: User \"system:serviceaccount:shop:"+
					"operator\" cannot get resource \"leases\" in API "+
					"group \"coordination.k8s.io\"")
			return containerOf(pod)
		}},
	{row: "account-replaced", want: "serviceaccount/shop/api",
		build: func(f *fixture) inventory.EntityID {
			pod := f.workload("shop", "api", 1)[0]
			account := inventory.CoreID(kube.KindAccount, "shop", "api")
			f.add(account)
			f.relate(pod, inventory.References, account)
			f.change(account, 1, "uid")
			f.fail(containerOf(pod), detection.ModeCrashLoop, failingH, 2,
				"error: You must be logged in to the server (Unauthorized)")
			return containerOf(pod)
		}},
	{row: "join-token-expired",
		want: "secret/kube-system/bootstrap-token-abcdef",
		build: func(f *fixture) inventory.EntityID {
			pod := f.workload("shop", "web", 1)[0]
			token := inventory.CoreID(kube.KindSecret, "kube-system",
				"bootstrap-token-abcdef")
			f.apply(inventory.Observation{Kind: inventory.Observed,
				Entity: token, Attributes: map[string]inventory.Value{
					kube.AttrTokenExpiry: inventory.Time(
						t0.Add(-24 * time.Hour))}})
			f.fail(pod, detection.ModeUnschedulable, failingH, 2,
				"0/2 nodes are available: 2 Insufficient cpu.")
			return pod
		}},
}
