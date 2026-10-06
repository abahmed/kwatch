package compose

import (
	"sort"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// sortByImpact orders decisions so the list a reader skims starts with
// what matters most: the loudest tier, then incidents that lose traffic,
// then the larger impact, then the name so the order is stable. It
// sorts in place; callers pass their own copy.
func sortByImpact(decisions []incident.Decision) {
	sort.SliceStable(decisions, func(i, j int) bool {
		a, b := decisions[i].Incident, decisions[j].Incident
		switch {
		case a.Tier != b.Tier:
			return a.Tier > b.Tier
		case losesTraffic(a) != losesTraffic(b):
			return losesTraffic(a)
		case len(a.Impact) != len(b.Impact):
			return len(a.Impact) > len(b.Impact)
		}
		return a.Root.String() < b.Root.String()
	})
}

// losesTraffic reports an incident whose requests fail: the manager
// found a routed Service without backends, or a Service or Ingress is in
// the impact, which the messages already describe as unable to serve.
func losesTraffic(inc incident.Incident) bool {
	if inc.TrafficLost() {
		return true
	}
	for _, id := range inc.Impact {
		if id.Kind == kube.KindService || id.Kind == kube.KindIngress {
			return true
		}
	}
	return false
}
