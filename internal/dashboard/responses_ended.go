package dashboard

import "time"

// DeliveryOutcome is what the alert channels hold of the incident: the
// state of its alert at each pager and the text of the latest message
// each channel accepted. Each is absent without data.
type DeliveryOutcome struct {
	Pager []PagerRow    `json:"pager,omitempty"`
	Sent  []SentMessage `json:"sent,omitempty"`
}

// PagerRow is the incident's alert at one pager. State is open (paged,
// not resolved there), closed (the resolve landed), failed (the pager
// refused the page for good) or pending (still being retried). PagedAt
// and ClosedAt are absent until they happen; Reason is the last
// failure's plain reason, such as "token refused (401)".
type PagerRow struct {
	Provider string    `json:"provider"`
	Name     string    `json:"name"`
	State    string    `json:"state"`
	PagedAt  time.Time `json:"pagedAt,omitzero"`
	ClosedAt time.Time `json:"closedAt,omitzero"`
	Reason   string    `json:"reason,omitempty"`
}

// SentMessage is the latest message one channel accepted about the
// incident, as plain text of at most 2 KiB; Cut says the text was
// shortened.
type SentMessage struct {
	Provider string    `json:"provider"`
	Name     string    `json:"name"`
	At       time.Time `json:"at"`
	Kind     string    `json:"kind,omitempty"`
	Text     string    `json:"text"`
	Cut      bool      `json:"cut,omitempty"`
}

// DeliveryDetail is what the delivery side holds of one incident
// beyond its trail, for SetDeliveryDetail.
type DeliveryDetail struct {
	Pager []PagerRow
	Sent  []SentMessage
}

// IncidentEnd is how the incident ended and what the page cannot show.
//
// StillCrashing marks a stopped-tracking resolve (the list item's
// StoppedTracking) that held because a container kept crashing. Moved
// links the incidents it was moved into or that were moved into it,
// those kwatch still keeps. ChangedBeforeRecovery are the changes made
// shortly before a resolved incident recovered, newest first, at most
// two; it never says they fixed it. LogsNotReadable says the log
// section is empty because kwatch may not read pod logs.
type IncidentEnd struct {
	StillCrashing         bool         `json:"stillCrashing,omitempty"`
	Moved                 *MovedLinks  `json:"moved,omitempty"`
	ChangedBeforeRecovery []ChangeItem `json:"changedBeforeRecovery,omitempty"`
	LogsNotReadable       bool         `json:"logsNotReadable,omitempty"`
}

// MovedLinks are the incident this one was moved into and the earlier
// ones moved into it, oldest first.
type MovedLinks struct {
	Into *IncidentRef  `json:"into,omitempty"`
	From []IncidentRef `json:"from,omitempty"`
}

// IncidentRef names an incident that can still be opened.
type IncidentRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
