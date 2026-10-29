package investigate

import (
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
			if waitingReasons[reason] {
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
			if code == "" || code == "0" {
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
