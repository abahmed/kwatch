package investigate

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// waitingReasons are the waiting reasons the kubelet states as the
// cause itself: the image cannot be used or the container cannot be
// configured.
var waitingReasons = map[string]bool{
	"ErrImagePull": true, "ImagePullBackOff": true,
	"ErrImageNeverPull": true, "InvalidImageName": true,
	"CreateContainerConfigError": true, "CreateContainerError": true,
}

// confirmWaiting confirms a cause the kubelet itself reports as the
// container's waiting reason.
func confirmWaiting(
	model inventory.Reader, p incident.Incident, _ []incident.Section,
	_ signature,
) string {
	for _, pod := range incidentPods(model, p) {
		for _, c := range containersOf(model, pod) {
			e, _ := model.Entity(c)
			reason := textOf(e, kube.AttrStateReason)
			if waitingReasons[reason] &&
				waitSupports(model, p, pod, c, reason) {
				return "the kubelet reports " + c.Name + " waiting with " +
					reason
			}
		}
	}
	return ""
}

// confirmCrashExit confirms a crash from the run that ended it: a
// non-zero exit, and the first error the run logged when one was read.
func confirmCrashExit(
	model inventory.Reader, p incident.Incident, report []incident.Section,
	sig signature,
) string {
	if !isCrashRoot(p) {
		return ""
	}
	for _, pod := range incidentPods(model, p) {
		for _, c := range containersOf(model, pod) {
			e, _ := model.Entity(c)
			code := exitCodeOf(e)
			if code == "" || code == "0" || !runSupports(model, p, pod, c) {
				continue
			}
			why := "the last run of " + c.Name + " exited with code " + code
			if line := firstErrorOf(report, sig); line != "" {
				why += " after logging \"" + line + "\""
			}
			return why
		}
	}
	return ""
}

// runSupports reports that a run of container c in pod speaks for the
// incident's cause: the root runs application code, or the pod, the
// container or one of the pod's owners (a ReplicaSet, a Rollout, an
// operator's workload) is itself the root or the cause's root. An
// unrelated pod that crashed under a node, network or other
// infrastructure root is still shown as evidence, but proves nothing
// about that root.
func runSupports(
	model inventory.Reader, p incident.Incident, pod, c inventory.EntityID,
) bool {
	return incident.IsApplicationKind(p.Root.Kind) ||
		blamed(p, pod) || blamed(p, c) || ownerBlamed(model, p, pod)
}

// maxOwnerHops bounds the owner chain runSupports follows: pod,
// ReplicaSet, Deployment, and an operator's workload above it.
const maxOwnerHops = 4

// ownerBlamed reports an owner of pod, at any level of its chain, that
// is the root or the cause's root.
func ownerBlamed(
	model inventory.Reader, p incident.Incident, pod inventory.EntityID,
) bool {
	level := []inventory.EntityID{pod}
	for range maxOwnerHops {
		var next []inventory.EntityID
		for _, id := range level {
			next = append(next,
				model.Related(id, inventory.OwnedBy, inventory.Outgoing)...)
		}
		if slices.ContainsFunc(next, func(id inventory.EntityID) bool {
			return blamed(p, id)
		}) {
			return true
		}
		level = next
	}
	return false
}

// imageKinds and configKinds are the infrastructure roots a waiting
// reason names directly: an image that cannot be pulled, a ConfigMap or
// Secret the container cannot be configured from.
var (
	imageKinds  = []inventory.Kind{kube.KindImage, kube.KindRegistry}
	configKinds = []inventory.Kind{kube.KindConfigMap, kube.KindSecret}
)

// waitSupports is runSupports, widened to the image and config roots
// the waiting reason itself names.
func waitSupports(
	model inventory.Reader, p incident.Incident, pod, c inventory.EntityID,
	reason string,
) bool {
	if runSupports(model, p, pod, c) {
		return true
	}
	kinds := imageKinds
	if strings.HasPrefix(reason, "CreateContainer") {
		kinds = configKinds
	}
	return rootOrCauseIn(p, kinds)
}

// rootOrCauseIn reports the root or the cause's root is one of kinds.
func rootOrCauseIn(p incident.Incident, kinds []inventory.Kind) bool {
	if slices.Contains(kinds, p.Root.Kind) {
		return true
	}
	return p.Cause != nil && slices.Contains(kinds, p.Cause.Root.Kind)
}

// firstErrorOf is the classified failure line, else the first matched
// line of a log section.
func firstErrorOf(report []incident.Section, sig signature) string {
	if sig.line != "" {
		return sig.line
	}
	for _, s := range report {
		if s.Check == incident.CheckLogs || s.Check == incident.CheckCrash {
			if lines := matchedLines(s); len(s.Matched) > 0 {
				return clip(lines[0])
			}
		}
	}
	return ""
}

// exitCodeOf is the exit code of the run that ended last: the last
// termination once the container restarted, else its current state. A
// container that just crashed for the first time has no last
// termination yet; the kubelet reports that exit as its current state.
func exitCodeOf(e inventory.Entity) string {
	if code := textOf(e, kube.AttrLastExitCode); code != "" {
		return code
	}
	return textOf(e, kube.AttrExitCode)
}
