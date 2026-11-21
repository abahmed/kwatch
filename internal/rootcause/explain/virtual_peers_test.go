package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var peerRowCases = []rowCase{
	{row: "headless-hides-not-ready-peers", want: "service/shop/db",
		build: func(f *fixture) inventory.EntityID {
			svc := inventory.CoreID(kube.KindService, "shop", "db")
			f.apply(inventory.Observation{Kind: inventory.Observed,
				Entity: svc, Attributes: map[string]inventory.Value{
					kube.AttrHeadless: inventory.Bool(true)}})
			pods := f.workload("shop", "db", 2)
			for i, pod := range pods {
				f.fail(pod, detection.ModeNotReady, degradedH, 2,
					"readiness probe failed: cannot resolve db-"+
						string(rune('0'+i))+".db.shop.svc: NXDOMAIN")
				f.note(pod, "Readiness probe failed: cannot resolve "+
					"db-"+string(rune('0'+i))+".db.shop.svc: NXDOMAIN")
			}
			return pods[0]
		}},
}
