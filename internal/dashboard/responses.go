package dashboard

import (
	"time"
)

// ListResponse is the /api/incidents response, newest first. Total is
// how many incidents matched; Truncated is true when the limit or the
// response size left some out.
type ListResponse struct {
	Incidents []IncidentListItem `json:"incidents"`
	Total     int                `json:"total"`
	Truncated bool               `json:"truncated"`
}

// IncidentListItem is one incident in the /api/incidents list.
type IncidentListItem struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Severity    string    `json:"severity"`
	State       string    `json:"state"`
	Namespace   string    `json:"namespace"`
	Workload    string    `json:"workload"`
	FirstSeen   time.Time `json:"firstSeen"`
	LastSeen    time.Time `json:"lastSeen"`
	RepeatCount int       `json:"repeatCount"`
	AckedBy     string    `json:"ackedBy,omitempty"`
	ResolvedAt  time.Time `json:"resolvedAt,omitzero"`
}

// IncidentDetail is one incident's full detail for /api/incidents/{id}.
// Kubernetes events and per-channel delivery attempts are not held per
// incident, so the detail does not carry them.
type IncidentDetail struct {
	IncidentListItem
	RootCause        string   `json:"rootCause"`
	SuggestedFix     string   `json:"suggestedFix,omitempty"`
	Timeline         []Event  `json:"timeline"`
	CapturedLogLines []string `json:"capturedLogLines"`
	DocsLink         string   `json:"docsLink,omitempty"`
}

// Event is one timeline entry.
type Event struct {
	At     time.Time `json:"at"`
	Text   string    `json:"text"`
	Entity string    `json:"entity,omitempty"`
}

// StatusResponse is the /api/status response. WatchedNamespaces is ["*"]
// when every namespace is watched.
type StatusResponse struct {
	Version            string          `json:"version"`
	Edition            string          `json:"edition"`
	LastScan           time.Time       `json:"lastScan"`
	WatchedNamespaces  []string        `json:"watchedNamespaces"`
	ExcludedNamespaces []string        `json:"excludedNamespaces,omitempty"`
	PerChannelHealth   []ChannelHealth `json:"perChannelHealth"`
}

// ChannelHealth is one alert channel's delivery health. LastErrorMessage
// is short and redacted; it never carries a webhook URL or token.
type ChannelHealth struct {
	Channel          string    `json:"channel"`
	LastSuccess      time.Time `json:"lastSuccess,omitzero"`
	LastError        time.Time `json:"lastError,omitzero"`
	LastErrorMessage string    `json:"lastErrorMessage,omitempty"`
}
