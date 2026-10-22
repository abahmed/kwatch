package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The Pro line of a Community message: one line at most, either the
// teaser (a Pro analysis would explain an incident Community leaves
// without a cause) or the note for the alert's type. Copy is fixed by
// kwatch.dev/packages/brand/specs/community-pro-teasers.md: no names,
// no interpolation.

// proInvite closes every Pro line.
const proInvite = " Get Pro free during early access: kwatch.dev/pro"

// proTeaser replaces the note when Pro found a likely cause.
const proTeaser = "Pro found a likely cause for this."

// Notes by alert type.
const (
	noteCrash = "Pro would show the change that caused this crash."
	noteOOM   = "Pro would show what changed memory use and the " +
		"limit to set."
	noteImage   = "Pro would show who changed the image and when."
	notePending = "Pro would show which nodes were ruled out and why."
	noteNode    = "Pro would show which pods this node took down."
	noteGeneric = "Pro would show the recent change behind this and " +
		"the services it affects."
)

// proNote is the one Pro line of d, or "" when the engine found nothing
// Pro would show.
func proNote(d incident.Decision) string {
	if proTeaserFits(d) {
		return proTeaser + proInvite
	}
	if !proFound(d) {
		return ""
	}
	return noteByType(d.Incident) + proInvite
}

// proTeaserFits reports an incident without a cause that a withheld
// analysis would explain: a shared factor, or a recent change. With a
// cause the one-line cause already names it.
func proTeaserFits(d incident.Decision) bool {
	if d.Incident.Cause != nil {
		return false
	}
	return d.Incident.ProCause || len(d.Facts.Changes) > 0 ||
		d.Facts.Window != nil
}

// proFound reports whether the engine found something Pro would show:
// a recent change, affected services, or logs.
func proFound(d incident.Decision) bool {
	cause := d.Incident.Cause
	return (cause != nil && cause.Change != nil) ||
		len(d.Facts.Changes) > 0 || d.Facts.Window != nil ||
		len(d.Incident.Impact) > 0 ||
		(cause != nil && len(cause.Hops) > 0) ||
		hasLogs(d.Incident.Root.Kind)
}

// noteByType is the note of the incident's alert type.
func noteByType(p incident.Incident) string {
	if p.Root.Kind == kube.KindNode && p.Mode == detection.ModeNotReady {
		return noteNode
	}
	switch p.Mode {
	case detection.ModeCrashLoop, detection.ModeError,
		detection.ModeRestarting, detection.ModeProbe,
		detection.ModeProbeStartup:
		return noteCrash
	case detection.ModeOOMKilled:
		return noteOOM
	case detection.ModeImagePull, detection.ModeImagePullInspect,
		detection.ModeImagePullInvalidName,
		detection.ModeImagePullRegistry:
		return noteImage
	case detection.ModePending, detection.ModeUnschedulable:
		return notePending
	}
	return noteGeneric
}

// hasLogs reports a root whose container output Pro would quote.
func hasLogs(kind inventory.Kind) bool {
	return kind == kube.KindPod || kind == kube.KindContainer
}
