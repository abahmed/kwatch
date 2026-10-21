package detection

import (
	"fmt"
	"slices"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ReasonInfo is everything kwatch knows about one finding reason, in one
// place: the failure mode it maps to, whether it is a failure by itself
// and how serious, whether it only waits for the digest, whether the
// object state already shows it, and a plain-words label. A new reason
// is one entry in the reason tables (reason_table_*.go).
type ReasonInfo struct {
	Reason string
	Mode   Mode
	// Waiting is the severity of a container waiting state that is a
	// failure. Zero means the state is not one.
	Waiting Severity
	// Event is the severity of a Warning event that is a failure on its
	// own. Zero means the event is not one.
	Event Severity
	// Progress marks a waiting state that is normal progress, such as a
	// container being created. Every other waiting reason that is not
	// a failure here is unknown, and an unknown one that lasts is a
	// finding (see UnknownWaiting).
	Progress bool
	// Digest marks a finding that never interrupts on its own.
	Digest bool
	// CauseOnly marks a finding that is a cause but no news alone.
	CauseOnly bool
	// ShownByState marks a Warning event reason whose condition is
	// already read from the object itself.
	ShownByState bool
	// ShownOn lists the kinds whose own state shows the reason. The same
	// reason on another kind (a custom resource's controller may reuse
	// "Failed" or "BackOff") is not covered by that state. Empty means
	// every kind.
	ShownOn []inventory.Kind
	// Label says in a few plain words what the reason means for the
	// object it is about.
	Label string
}

// reason starts an entry that names the mode of a reason.
func reason(name string, mode Mode) ReasonInfo {
	return ReasonInfo{Reason: name, Mode: mode}
}

// eventReason starts an entry for a Warning event that has no mode of
// its own and is a failure of the given severity.
func eventReason(name string, severity Severity) ReasonInfo {
	return ReasonInfo{Reason: name, Event: severity}
}

// stateReason starts an entry for a Warning event reason the state of
// the given kinds already shows.
func stateReason(name string, kinds ...inventory.Kind) ReasonInfo {
	return ReasonInfo{Reason: name, ShownByState: true, ShownOn: kinds}
}

// waiting marks a container waiting state that is a failure.
func (i ReasonInfo) waiting(severity Severity) ReasonInfo {
	i.Waiting = severity
	return i
}

// event marks a Warning event that is a failure on its own.
func (i ReasonInfo) event(severity Severity) ReasonInfo {
	i.Event = severity
	return i
}

// progress marks a waiting state that is normal progress.
func (i ReasonInfo) progress() ReasonInfo {
	i.Progress = true
	return i
}

// digest marks a reason that waits for the digest.
func (i ReasonInfo) digest() ReasonInfo {
	i.Digest = true
	return i
}

// causeOnly marks a reason that is a cause but no news alone.
func (i ReasonInfo) causeOnly() ReasonInfo {
	i.CauseOnly = true
	return i
}

// shown marks a reason whose condition the state of the given kinds
// already shows.
func (i ReasonInfo) shown(kinds ...inventory.Kind) ReasonInfo {
	i.ShownByState = true
	i.ShownOn = kinds
	return i
}

// label sets the plain-words meaning.
func (i ReasonInfo) label(text string) ReasonInfo {
	i.Label = text
	return i
}

// reasonTable is every ReasonInfo, keyed by reason. The per-attribute
// lookups below read it, so no attribute is listed anywhere else.
var reasonTable = buildReasonTable(
	podReasons, nodeReasons, workloadReasons, serviceReasons,
	eventOnlyReasons, upstreamReasons,
	shownOn(admissionReasons, kube.KindPod),
)

// shownOn gives every entry of group the kinds that show it, for a group
// whose reasons all concern one kind.
func shownOn(group []ReasonInfo, kinds ...inventory.Kind) []ReasonInfo {
	out := make([]ReasonInfo, len(group))
	for i, info := range group {
		info.ShownOn = kinds
		out[i] = info
	}
	return out
}

// buildReasonTable indexes the entries and fails fast on a reason that
// is listed twice, since the second entry would hide the first.
func buildReasonTable(groups ...[]ReasonInfo) map[string]ReasonInfo {
	out := make(map[string]ReasonInfo)
	for _, group := range groups {
		for _, info := range group {
			if _, dup := out[info.Reason]; dup {
				panic(fmt.Sprintf("reason %q is listed twice", info.Reason))
			}
			out[info.Reason] = info
		}
	}
	return out
}

// modeByReason names the failure mode of each reason. Modes group reasons
// that describe one condition (both OOM spellings, the HPA metric event
// reasons) and are what the tracker compares to decide whether a finding
// changed. The values are stable: changing one re-announces incidents.
var modeByReason = modesOf(reasonTable)

// modesOf lists the mode of every entry that names one.
func modesOf(table map[string]ReasonInfo) map[string]Mode {
	out := make(map[string]Mode)
	for name, info := range table {
		if info.Mode != "" {
			out[name] = info.Mode
		}
	}
	return out
}

// WaitingSeverity is the severity of a container waiting state that is a
// failure; ok is false for states that are progress, not failure.
func WaitingSeverity(reason string) (severity Severity, ok bool) {
	severity = reasonTable[reason].Waiting
	return severity, severity != 0
}

// EventSeverity is the severity of a Warning event that is a failure on
// its own; ok is false when the event is not one.
func EventSeverity(reason string) (severity Severity, ok bool) {
	severity = reasonTable[reason].Event
	return severity, severity != 0
}

// ShownByState reports a Warning event reason whose condition kwatch
// already reads from the object itself, on some kind.
func ShownByState(reason string) bool {
	return reasonTable[reason].ShownByState
}

// ShownByStateOn reports a Warning event reason of an object of kind
// whose condition kwatch already reads from that object's own state.
// The same reason on another kind is not shown by anything.
func ShownByStateOn(reason string, kind inventory.Kind) bool {
	info := reasonTable[reason]
	return info.ShownByState &&
		(len(info.ShownOn) == 0 || slices.Contains(info.ShownOn, kind))
}

// DigestOnly reports a reason that never interrupts on its own.
func DigestOnly(reason string) bool {
	return reasonTable[reason].Digest
}

// CauseOnly reports a reason that is a real cause but no news alone.
func CauseOnly(reason string) bool {
	return reasonTable[reason].CauseOnly
}

// ReasonLabel says in plain words what a reason means for its object.
// It is empty for a reason without a label.
func ReasonLabel(reason string) string {
	return reasonTable[reason].Label
}

// WaitingProgress reports a container waiting state that is normal
// progress, not a failure.
func WaitingProgress(reason string) bool {
	return reasonTable[reason].Progress
}

// KnownReason reports whether the reason has an entry in the registry.
func KnownReason(reason string) bool {
	_, ok := reasonTable[reason]
	return ok
}

// UnknownWaiting reports a container waiting reason the registry does
// not classify: neither a failure nor progress. Such a state is
// never assumed healthy; a detector reports it once it has lasted.
func UnknownWaiting(reason string) bool {
	info := reasonTable[reason]
	return reason != "" && info.Waiting == 0 && !info.Progress
}
