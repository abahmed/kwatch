package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// leaseRows blame a failing holder pod for the Lease it stopped
// renewing. The Lease is linked to its holder by the prober.
var leaseRows = []Row{
	{
		Name: "lease-holder-failing",
		Cause: Side{Kind: kube.KindPod,
			Modes: append([]detection.Mode{detection.ModeNotReady,
				detection.ModePending}, podFailures...)},
		Link: LinkUses,
		Effect: Side{Kind: kube.KindLease,
			Modes: []detection.Mode{detection.ModeLeaseStale}},
		Prior: 0.8,
	},
}
