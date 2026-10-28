package dashboard

import (
	"time"
)

// ListResponse is the /api/incidents response, newest first. Total is
// how many incidents matched; Truncated is true when the limit or the
// response size left some out.
//
// Counts is how many incidents of the requested state there are by
// severity, before the other filters and the limit: the nav badge and
// the "show the filters" decision read it.
type ListResponse struct {
	Incidents []IncidentListItem `json:"incidents"`
	Total     int                `json:"total"`
	Truncated bool               `json:"truncated"`
	Counts    map[string]int     `json:"counts"`
}

// NamespacesResponse is the /api/namespaces response: the namespaces
// that have incidents of the requested state, A to Z.
type NamespacesResponse struct {
	Namespaces []string `json:"namespaces"`
}

// IncidentListItem is one incident in the /api/incidents list.
type IncidentListItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Severity  string `json:"severity"`
	State     string `json:"state"`
	Namespace string `json:"namespace"`
	Workload  string `json:"workload"`
	// WorkloadKind is the kind of the root object: Deployment, Node.
	WorkloadKind string    `json:"workloadKind,omitempty"`
	FirstSeen    time.Time `json:"firstSeen"`
	LastSeen     time.Time `json:"lastSeen"`
	RepeatCount  int       `json:"repeatCount"`
	AckedBy      string    `json:"ackedBy,omitempty"`
	ResolvedAt   time.Time `json:"resolvedAt,omitzero"`
}

// IncidentDetail is one incident's full detail for /api/incidents/{id}.
// Kubernetes events and per-channel delivery attempts are not held per
// incident, so the detail does not carry them.
//
// SuggestedFix is the sentences of the last message's steps; FixSteps
// are the same steps with their exact commands, and FixCommands the
// commands alone, for copy buttons.
type IncidentDetail struct {
	IncidentListItem
	RootCause        string    `json:"rootCause"`
	SuggestedFix     string    `json:"suggestedFix,omitempty"`
	FixSteps         []FixStep `json:"fixSteps"`
	FixCommands      []string  `json:"fixCommands"`
	Timeline         []Event   `json:"timeline"`
	CapturedLogLines []string  `json:"capturedLogLines"`
	DocsLink         string    `json:"docsLink,omitempty"`
}

// FixStep is one suggested step: a sentence and, when the step is a
// command, the exact command.
type FixStep struct {
	Text    string `json:"text,omitempty"`
	Command string `json:"command,omitempty"`
}

// Event is one timeline entry. Kind is a stable code for what happened
// (see eventKinds); Reason refines a "resolved" event.
type Event struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Reason string    `json:"reason,omitempty"`
	Text   string    `json:"text"`
	Entity string    `json:"entity,omitempty"`
}

// StatusResponse is the /api/status response. WatchedNamespaces is ["*"]
// when every namespace is watched. LastScan is the engine's latest loop
// iteration, zero before the first; LastScanLate says it is older than
// kwatch allows. KwatchNamespace and KwatchDeployment name kwatch's own
// Deployment, for the command that reads its logs.
type StatusResponse struct {
	Version            string          `json:"version"`
	Edition            string          `json:"edition"`
	LastScan           time.Time       `json:"lastScan,omitzero"`
	LastScanLate       bool            `json:"lastScanLate"`
	ScanEverySeconds   int             `json:"scanEverySeconds"`
	KwatchNamespace    string          `json:"kwatchNamespace"`
	KwatchDeployment   string          `json:"kwatchDeployment"`
	WatchedNamespaces  []string        `json:"watchedNamespaces"`
	ExcludedNamespaces []string        `json:"excludedNamespaces,omitempty"`
	PerChannelHealth   []ChannelHealth `json:"perChannelHealth"`
}

// ChannelHealth is one alert channel's delivery health. LastErrorMessage
// is short and redacted; it never carries a webhook URL or token.
//
// Type is the provider (slack, email); Name is how the dashboard shows
// it. Neither is ever a URL or a token.
type ChannelHealth struct {
	Channel          string    `json:"channel"`
	Type             string    `json:"type"`
	Name             string    `json:"name"`
	LastSuccess      time.Time `json:"lastSuccess,omitzero"`
	LastError        time.Time `json:"lastError,omitzero"`
	LastErrorMessage string    `json:"lastErrorMessage,omitempty"`
}
