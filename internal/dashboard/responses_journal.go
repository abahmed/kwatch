package dashboard

import "time"

// The incident's own record of its life (incident.Journal), as the
// detail serves it. Each part is absent when kwatch has none, as for an
// incident saved by an older kwatch.

// BreakSnapshot is the incident's failing containers at one moment.
// Reason is "first break" or "got worse"; the rows are as they were
// then, while Containers are the pods as they are now.
type BreakSnapshot struct {
	At         time.Time      `json:"at"`
	Reason     string         `json:"reason"`
	Containers []ContainerRow `json:"containers"`
}

// Transition is one change of the incident's state or tier. Kind is
// opened, announced, recovering, failing again, flapping, reopened,
// resolved, state changed, tier raised or tier lowered; From and To are
// state names, or tier names for a tier change.
type Transition struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"`
	From string    `json:"from,omitempty"`
	To   string    `json:"to,omitempty"`
}

// WhyDetail says why the incident is at its alert level and why it is
// still open. HoldTotal is how long it must stay healthy (stable, when
// it flaps) before it closes and HoldElapsed how long it has, both in
// seconds. CannotSee says kwatch cannot observe the root right now, so
// the incident stays open with no hold running until it can check
// again.
type WhyDetail struct {
	AlertLevelReason string `json:"alertLevelReason,omitempty"`
	StillOpenReason  string `json:"stillOpenReason,omitempty"`
	HoldTotal        int    `json:"holdTotal,omitempty"`
	HoldElapsed      int    `json:"holdElapsed,omitempty"`
	CannotSee        bool   `json:"cannotSee,omitempty"`
	// PagingHeldReason says a page waits because the cluster is
	// scaling ("Paging held: cluster is scaling"); empty otherwise.
	PagingHeldReason string `json:"pagingHeldReason,omitempty"`
	// PagingHeldUntil is when the hold ends if no node changes again,
	// the Overview's scalingHold.expectedEnd; zero without a hold.
	PagingHeldUntil time.Time `json:"pagingHeldUntil,omitzero"`
	// UnverifiedReason says kwatch has held a finding of the incident
	// for a while over a kind it cannot read now ("kwatch can't verify
	// this right now: ..."); empty otherwise.
	UnverifiedReason string `json:"unverifiedReason,omitempty"`
}

// AckDetail is the acknowledgement of the incident: the person's note,
// when kwatch saw it, the tool that wrote it when the change history
// recorded the edit, and the object that carries it.
type AckDetail struct {
	Note  string    `json:"note,omitempty"`
	Since time.Time `json:"since,omitzero"`
	By    string    `json:"by,omitempty"`
	On    string    `json:"on"`
}

// FieldDiff is one changed field of a change, for the diff view. The
// data of a Secret or a ConfigMap names its key only: After is then
// added, changed or removed, never a value.
type FieldDiff struct {
	Path   string `json:"path"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}
