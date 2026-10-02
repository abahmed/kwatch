package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// trafficRows cover how requests reach pods:
// routes to their backend Services, and certificates to the pods and
// routes that serve or check them. A Service or route broken by its own
// edit is the generic own-change row.
var trafficRows = []Row{
	{
		// An Ingress or a Gateway API route that sends traffic to a
		// Service that does not exist. A route reports it through the
		// generic custom-resource finding (NotReconciling, "Reports
		// ResolvedRefs=False"); the missing Service it routes to is
		// the cause, even when the route's own edit named it.
		Name: "routed-missing",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{ModeMissing}},
		Link: LinkRoutesTo,
		Effect: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{
				detection.ModeBackendMissing,
				detection.ConditionModeOf("ResolvedRefs"),
				detection.ConditionModeOf("Accepted"),
				detection.ModeNotReconciling}},
		Prior: 0.75,
	},
	{
		// An expired certificate breaks every TLS handshake that uses
		// it: the pods that serve it and the routes that terminate
		// with it. Pods must say so in their errors; an Ingress has no
		// text of its own, its only reference is the Secret.
		Name: "certificate-expired",
		Cause: Side{Kind: kube.KindSecret,
			Modes: []detection.Mode{detection.ModeCertExpired}},
		Link: LinkUses,
		Effect: Side{Kind: kube.KindPod, Signal: SignalTLS,
			Modes: append([]detection.Mode{
				detection.ModeFailed, detection.ModeError}, podFailures...)},
		Prior: 0.8,
	},
}
