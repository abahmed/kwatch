package compose

import (
	"slices"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The Pro line of a Community message: one sentence at most, on the
// first alert of an issue only, saying what Pro shows: the teaser (a Pro
// analysis would explain an incident Community leaves without a cause)
// or the note for the alert's type, then the fix hint when Pro says how
// to fix it. A message that tells its cause's category has the line of
// causeNote instead (cause_category.go). The phrases are fixed copy: no
// names, no interpolation.

// proInvite closes every Pro line.
const proInvite = ", free during early access: kwatch.dev/pro"

// proTeaser replaces the note when Pro found a likely cause: a shared
// factor its analysis found (Incident.ProCause).
const proTeaser = "the likely cause it found"

// proChangeSeen replaces the note when the engine found no cause but
// saw a recent change next to the failure.
const proChangeSeen = "what changed just before this"

// proFixWords say that Pro has the fix Community leaves out: a Fix line
// or a step that changes the cluster. proCheckWords say that it has
// only a command to look closer. Either joins the note in its one line.
const (
	proFixWords   = "how to fix it"
	proCheckWords = "how to check it"
)

// Notes by alert type: what Pro shows for it. The notes that promise a
// change are used only when the engine found one (changeKnown).
var (
	noteCrash   = []string{"the change that caused this crash"}
	noteOOM     = []string{"what changed memory use", "the limit to set"}
	noteImage   = []string{"who changed the image and when"}
	notePending = []string{"which nodes were ruled out and why"}
	noteNode    = []string{"which pods this node took down"}
	noteGeneric = []string{"the recent change behind this",
		"the services it affects"}
	noteLog    = "the error from its log"
	noteImpact = "the services it affects"
)

// proHelp is what Pro gives to act on: a fix, or only a way to check.
type proHelp uint8

const (
	helpNone proHelp = iota
	helpCheck
	helpFix
)

// proNote is the one Pro line of d, or "" when the engine found nothing
// Pro would show. help adds how to fix it, or how to check it.
func proNote(d incident.Decision, help proHelp) string {
	var shows []string
	switch {
	case d.Incident.Cause == nil && d.Incident.ProCause:
		shows = append(shows, proTeaser)
	case d.Incident.Cause == nil && changeKnown(d):
		shows = append(shows, proChangeSeen)
	case proFound(d):
		shows = append(shows, typeNote(d)...)
	}
	switch help {
	case helpFix:
		shows = append(shows, proFixWords)
	case helpCheck:
		shows = append(shows, proCheckWords)
	}
	if len(shows) == 0 {
		return ""
	}
	return proLineText(shows)
}

// proLineText is the one sentence of a Pro line: "Pro shows which one
// and how to fix it, free during early access: kwatch.dev/pro".
func proLineText(shows []string) string {
	return "Pro shows " + joinWords(shows) + proInvite
}

// changeKnown reports a change the engine found for d: the cause's
// change, recent changes next to it, or changes in its health window.
func changeKnown(d incident.Decision) bool {
	cause := d.Incident.Cause
	return (cause != nil && cause.Change != nil) ||
		len(d.Facts.Changes) > 0 || d.Facts.Window != nil
}

// proFound reports whether the engine found something Pro would show:
// a recent change, affected services, or logs.
func proFound(d incident.Decision) bool {
	cause := d.Incident.Cause
	return changeKnown(d) || len(d.Incident.Impact) > 0 ||
		(cause != nil && len(cause.Hops) > 0) ||
		hasLogs(d.Incident.Root.Kind)
}

// typeNote is what Pro shows for d: the note of its alert type, or, when
// that note promises a change the engine did not find, what Pro does
// show: the error from its log and the services it affects.
func typeNote(d incident.Decision) []string {
	note := noteByType(d.Incident)
	if changeKnown(d) || !promisesChange(note) {
		return note
	}
	var out []string
	if len(d.Facts.Output) > 0 {
		out = append(out, noteLog)
	}
	if len(d.Incident.Impact) > 0 {
		out = append(out, noteImpact)
	}
	return out
}

// promisesChange reports a note that names a change.
func promisesChange(note []string) bool {
	for _, changed := range [][]string{noteCrash, noteOOM, noteImage,
		noteGeneric} {
		if slices.Equal(note, changed) {
			return true
		}
	}
	return false
}

// noteByType is what Pro shows for the incident's alert type.
func noteByType(p incident.Incident) []string {
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
