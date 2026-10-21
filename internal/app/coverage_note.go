package app

import (
	"fmt"
	"strings"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/health"
)

// coverageComponent is the /health entry that says some kinds are not
// watched for lack of permission. It reads "degraded" but is available:
// monitoring runs, it only sees less than the cluster holds.
const coverageComponent = "coverage"

// maxNotedKinds is how many kind names the startup warning lists.
const maxNotedKinds = 8

// coverageNote tells the operator, once, which kinds the first complete
// discovery could not watch because RBAC refuses them. With the chart's
// default rbac.mode=least-privilege that is every custom resource, and
// nothing else says so: detectors for those kinds stay silent and the
// permission audit only knows the built-in kinds.
type coverageNote struct {
	done bool
}

// observe looks at the published coverage. It does nothing until
// discovery is complete, and after the first complete pass it never
// reports again.
func (n *coverageNote) observe(
	sink sourceStatusSink, coverage health.CoverageSummary,
) {
	if n.done || !coverage.Complete {
		return
	}
	n.done = true
	total := coverage.Reasons[health.CoveragePermissionDenied]
	if total == 0 {
		return
	}
	klog.Warning(coverageWarning(total, coverage.UnavailableKinds))
	if sink != nil {
		sink.SetComponentStatus(coverageComponent, "degraded",
			"optional_permission_denied", true)
	}
}

// coverageWarning words the one warning: how many kinds are not watched,
// which ones, and the way to watch them.
func coverageWarning(total int, kinds []health.CoverageKind) string {
	var names []string
	for _, k := range kinds {
		if k.Reason == health.CoveragePermissionDenied {
			names = append(names, k.Kind)
		}
	}
	shown := names
	if len(shown) > maxNotedKinds {
		shown = shown[:maxNotedKinds]
	}
	list := strings.Join(shown, ", ")
	if more := total - len(shown); more > 0 {
		list = fmt.Sprintf("%s and %d more", list, more)
	}
	return fmt.Sprintf("%d kinds not watched (permission_denied): %s; "+
		"set rbac.mode=full to watch custom resources", total, list)
}
