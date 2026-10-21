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
	reasons.MemoryPressure:     "is no longer low on memory",
	reasons.NodeMemoryPressure: "is no longer low on memory",
	reasons.DiskPressure:       "is no longer low on disk space",
	reasons.PIDPressure:        "is no longer under process pressure",
	reasons.NetworkUnavailable: "has its network back",
	reasons.NodeFilesystemHigh: "is no longer low on disk space",
	reasons.NodeInodesHigh:     "is no longer low on inodes",
	reasons.NodeNetworkErrors:  "no longer has network errors",
	reasons.NodeRuntimeErrors:  "no longer has runtime errors",
}

// nodeModePhrases say what ended for an eviction threshold, which the
// kubelet names by the resource in the finding's mode.
var nodeModePhrases = map[detection.Mode]string{
	detection.ModeMemoryEvictionThreshold: "is no longer low on memory",
	detection.ModeDiskEvictionThreshold:   "is no longer low on disk space",
	detection.ModePIDEvictionThreshold:    "is no longer short of process IDs",
}

// nodeRecovered says how a node incident ended, by the failure that
// ended: a node that was NotReady "is ready again", one that was under
// CPU pressure "is no longer under CPU pressure", one that was low on
// memory "is no longer low on memory". It never claims readiness for a
// node that was never down.
func nodeRecovered(f caseFacts) string {
	ended := endedFindings(f)
	if len(ended) == 0 {
		return "is ready again"
	}
	for _, m := range ended {
		if nodeDownReasons[m.Reason] {
			return "is ready again"
		}
	}
	for _, m := range ended {
		if resource := stalledResource(m); resource != "" {
			return "is no longer under " + resource + " pressure"
		}
		if phrase := nodeModePhrases[m.Mode]; phrase != "" {
			return phrase
		}
		if phrase := nodePhrases[m.Reason]; phrase != "" {
			return phrase
		}
	}
	return "has recovered"
}

// endedFindings are the node findings the incident ended: the failing
// members, or, once they are gone at the resolve, the root findings the
// cause kept and the reasons the incident remembers.
func endedFindings(f caseFacts) []detection.Finding {
	if failures := failing(f.members); len(failures) > 0 {
		return failures
	}
	var out []detection.Finding
	if f.p.Cause != nil {
		out = append(out, f.p.Cause.RootFindings...)
	}
	for _, reason := range f.p.RootReasons() {
		out = append(out, detection.Finding{Reason: reason})
	}
	return out
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
