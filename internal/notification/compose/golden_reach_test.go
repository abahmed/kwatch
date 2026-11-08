package compose

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// reachCases are bad rollouts that cut users off from addresses an
// Ingress publishes.
func reachCases() []goldenCase {
	storefront := inventory.CoreID(kube.KindIngress, "shop", "storefront")
	reach := func(host, path string, ready, total int) incident.Reach {
		return incident.Reach{Host: host, Path: path, Source: storefront,
			Ready: ready, Total: total}
	}
	return []goldenCase{
		{"user_endpoints", func() notification.Message {
			return writeReach(
				reach("shop.example.com", "/checkout", 0, 3),
				reach("shop.example.com", "", 0, 3),
				reach("*.shop.example.com", "", 0, 3),
				reach("pay.example.com", "", 0, 3))
		}},
		{"user_endpoints_partial", func() notification.Message {
			return writeReach(
				reach("shop.example.com", "/checkout", 1, 3))
		}},
	}
}

func writeReach(reach ...incident.Reach) notification.Message {
	d := announce(badRollout())
	d.Facts.Reach = reach
	return Writer{}.Write(d, at(5, 0))
}
