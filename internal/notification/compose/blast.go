package compose

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// blastVerbs say how a pod depends on the object by its kind: "9 of 10
// failing pods read it". A ConfigMap is read, a Secret used, a claim
// mounted.
var blastVerbs = map[inventory.Kind]string{
	kube.KindConfigMap: "read",
	kube.KindSecret:    "use",
	kube.KindPVC:       "mount",
}

// blastSentences give the share of the failing pods that depend on the
// changed object, right after the change: "9 of 10 failing pods read
// it." It is told only when several changes preceded the failure (the
// proof exists for no other), with at least two pods and half of them.
func blastSentences(f caseFacts) []sentence {
	cause := f.p.Cause
	if cause == nil || cause.Rival != nil {
		// Two causes in one hedge: "it" would name neither.
		return nil
	}
	verb, ok := blastVerbs[cause.Root.Kind]
	if !ok {
		return nil
	}
	for _, item := range cause.Proof {
		if item.Code != rootcause.ProofBlastOverlap || !item.Supports ||
			item.Total < 2 || item.Count*2 < item.Total {
			continue
		}
		return []sentence{{part: partChanges, weight: 0.5,
			text: strconv.Itoa(item.Count) + " of " +
				strconv.Itoa(item.Total) + " failing pods " + verb +
				" it."}}
	}
	return nil
}
