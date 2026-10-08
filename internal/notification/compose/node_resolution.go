package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// nodeDownReasons are the findings that mean the node itself stopped
// working: only their end reads "is ready again".
var nodeDownReasons = map[string]bool{
	reasons.NodeNotReady: true, reasons.NotReady: true,
	reasons.NodeHeartbeatStale: true, reasons.NodeCNINotReady: true,
	reasons.NodeStuckTerminating: true,
}

// nodePhrases say what ended for the other node findings, after "is no
// longer" or "has".
var nodePhrases = map[string]string{
	reasons.MemoryPressure:     "is no longer under memory pressure",
	reasons.NodeMemoryPressure: "is no longer under memory pressure",
	reasons.DiskPressure:       "is no longer under disk pressure",
	reasons.PIDPressure:        "is no longer under process pressure",
	reasons.NetworkUnavailable: "has its network back",
	reasons.NodeFilesystemHigh: "is no longer low on disk space",
	reasons.NodeInodesHigh:     "is no longer low on inodes",
	reasons.NodeNetworkErrors:  "no longer has network errors",
	reasons.NodeRuntimeErrors:  "no longer has runtime errors",
}

// nodeRecovered says how a node incident ended, by the failure that
// ended: a node that was NotReady "is ready again", one that was under
// CPU pressure "is no longer under CPU pressure". It never claims
// readiness for a node that was never down.
func nodeRecovered(f caseFacts) string {
	failures := failing(f.members)
	if len(failures) == 0 {
		return "is ready again"
	}
	for _, m := range failures {
		if nodeDownReasons[m.Reason] {
			return "is ready again"
		}
	}
	for _, m := range failures {
		if resource := stalledResource(m); resource != "" {
			return "is no longer under " + resource + " pressure"
		}
		if phrase := nodePhrases[m.Reason]; phrase != "" {
			return phrase
		}
	}
	return "has recovered"
}

// stalledResource reads the resource a pressure-stall finding named in its
// summary ("Node is under CPU pressure: ..."); empty for any other
// finding.
func stalledResource(m detection.Finding) string {
	if m.Reason != reasons.NodePSIHigh {
		return ""
	}
	rest, ok := strings.CutPrefix(m.Summary, "Node is under ")
	resource, _, found := strings.Cut(rest, " pressure")
	if !ok || !found {
		return "resource"
	}
	return resource
}
