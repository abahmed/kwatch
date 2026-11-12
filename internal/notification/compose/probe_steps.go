package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// probeLines are the lines "kubectl describe" prints a container's
// probes on.
var probeLines = map[string]string{
	reasons.LivenessProbeFailed:  "Liveness",
	reasons.ReadinessProbeFailed: "Readiness",
	reasons.StartupProbeFailed:   "Startup",
}

// probeCheckStep shows a failing probe as it is set: the controller's
// pod template ("kubectl describe deploy/web", which outlives the pod),
// or the pod itself when it has no controller that replaces it.
func probeCheckStep(ref podRef, s detection.Finding) []notification.Step {
	line, ok := probeLines[s.Reason]
	if !ok {
		return nil
	}
	target := ref.describe()
	if ref.owned() {
		target = replacedOwners[ref.owner.Kind] + "/" + quote(ref.owner.Name)
	}
	return []notification.Step{{
		Text: "Check its " + probeCheckWords[s.Reason] + " (the " + line +
			" line: path, port, delay and timeout)",
		Command: "kubectl describe " + target + ref.ns,
	}}
}

// stepMember is the member the symptom steps read: the first member,
// or a container of its pod that a probe keeps killing, as the probe
// lead names it (the pod's not-ready finding often comes first).
func stepMember(members []detection.Finding) detection.Finding {
	first := members[0]
	for _, m := range members {
		if (m.Reason == reasons.LivenessProbeFailed ||
			m.Reason == reasons.StartupProbeFailed) &&
			m.Entity.Namespace == first.Entity.Namespace &&
			podName(m.Entity) == podName(first.Entity) {
			return m
		}
	}
	return first
}

// podName is the pod a pod or container finding is about.
func podName(id inventory.EntityID) string {
	if id.Kind == kube.KindContainer {
		pod, _ := splitContainer(id.Name)
		return pod
	}
	return id.Name
}
