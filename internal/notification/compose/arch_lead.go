package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
)

// archLead names a container that cannot run on its node's CPU, with or
// without a release behind it: "api in shop can't run: its image
// api:1.4 is not built for this node's CPU (arm64)", or, while replicas
// on nodes of another CPU run fine, "api in shop can't run on arm64
// node n1: its image api:1.4 is not built for that CPU". The kernel's
// "exec format error" on a node of a known architecture (the arch
// detector's evidence) is the whole story; the quoted line and the
// replicas that run fine on other nodes follow as proof. A release
// whose previous version still serves keeps its own lead (startLead).
func archLead(f caseFacts) (string, bool) {
	for _, m := range f.members {
		node := evidence(m, detection.EvidenceArchNode)
		arch, _, ok := strings.Cut(node, " node ")
		if !ok || arch == "" {
			continue
		}
		image := "its image"
		ref := shortImage(evidence(m, detection.EvidenceArchImage))
		if ref != "" {
			image += " " + ref
		}
		name := f.leadName(affectedWorkload(f))
		if evidence(m, detection.EvidenceArchHealthy) != "" {
			// Replicas on other nodes run: only this node's CPU fails.
			return name + " can't run on " + node + ": " + image +
				" is not built for that CPU", true
		}
		return name + " can't run: " + image +
			" is not built for this node's CPU (" + arch + ")", true
	}
	return "", false
}

// shortImage is an image reference without its registry path:
// "api:1.4" for "ghcr.io/x/api:1.4".
func shortImage(ref string) string {
	ref = strings.TrimSpace(ref)
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		ref = ref[i+1:]
	}
	return ref
}
