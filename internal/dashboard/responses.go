package dashboard

import (
	"time"
)

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
	// Acked says someone acknowledged the incident; the detail's Ack
	// says more.
	Acked bool `json:"acked,omitempty"`
	// StoppedTracking marks a resolved incident kwatch stopped tracking
	// at its still-broken cap rather than one that recovered.
	StoppedTracking bool `json:"stoppedTracking,omitempty"`
	// Untold marks an incident no message reached: it settled,
	// recovered or moved before alerting. A page shows it as "Settled
	// before alerting"; counts of pages and notifications leave it out.
	Untold bool `json:"untold,omitempty"`
	// RootCause is the one-line cause, so a list row can say why
	// without opening the issue. Empty while kwatch has none.
	RootCause string `json:"rootCause"`
	// Rev fingerprints what a list row shows, so a page can tell which
	// rows changed between two reads. Only lists carry it.
	Rev string `json:"rev,omitempty"`
}

// IncidentDetail is one incident's full detail for /api/incidents/{id}.
// Kubernetes events are not held per incident, so the detail does not
// carry them.
//
// FixSteps are the last message's steps with their exact commands.
type IncidentDetail struct {
	IncidentListItem
	// Summary is the page's five-line top (summary.go); absent only
	// when the size limit left it out.
	Summary          *IncidentSummary `json:"summary,omitempty"`
	FixSteps         []FixStep        `json:"fixSteps"`
	Timeline         []Event          `json:"timeline"`
	CapturedLogLines []string         `json:"capturedLogLines"`
	DocsLink         string           `json:"docsLink,omitempty"`
	// Evidence is one section per check the last investigation ran (Pro
	// only; absent otherwise), and Correlation what kwatch concluded
	// across them.
	Evidence    []EvidenceSection `json:"evidence,omitempty"`
	Correlation *Correlation      `json:"correlation,omitempty"`
	// The incident page's context: what changed around it, earlier
	// occurrences, its containers now, who was told, its affected
	// objects and its states over time. Each is absent without data;
	// see responses_context.go.
	Changes    []ChangeItem      `json:"changes,omitempty"`
	FixedBy    *ChangeItem       `json:"fixedBy,omitempty"`
	Fix        string            `json:"fix,omitempty"`
	SeenBefore *SeenBefore       `json:"seenBefore,omitempty"`
	Containers []ContainerRow    `json:"containers,omitempty"`
	Deliveries []DeliveryAttempt `json:"deliveries,omitempty"`
	Affected   []AffectedObject  `json:"affected,omitempty"`
	Phases     []Phase           `json:"phases,omitempty"`
	// What the cluster says now about the incident's objects: recent
	// Warning events, the Services and Ingresses it affects, the nodes
	// involved, failing probes and why pods wait to be placed. See
	// responses_insight.go.
	Warnings []RecentWarning `json:"warnings,omitempty"`
	Traffic  []TrafficRow    `json:"traffic,omitempty"`
	Nodes    []NodeRow       `json:"nodes,omitempty"`
	Probes   []ProbeRow      `json:"probes,omitempty"`
	Waiting  *Waiting        `json:"waiting,omitempty"`
	// The incident's own record (responses_journal.go): its failing
	// containers when it broke and when it last got worse (Snapshot
	// is the latest of Snapshots), its state and tier changes, why it
	// is at its level and still open, and its acknowledgement.
	Snapshot    *BreakSnapshot  `json:"snapshot,omitempty"`
	Snapshots   []BreakSnapshot `json:"snapshots,omitempty"`
	Transitions []Transition    `json:"transitions,omitempty"`
	Why         *WhyDetail      `json:"why,omitempty"`
	Ack         *AckDetail      `json:"ack,omitempty"`
	// Timing, SameTime and UsageHours place the incident in time
	// (responses_view.go): when it started, was noticed and told, the
	// other issues that started with it, and the memory or CPU peaks
	// of its containers when they are evidence.
	Timing     *Timing        `json:"timing,omitempty"`
	SameTime   []SameTimeItem `json:"sameTime,omitempty"`
	UsageHours []UsageHistory `json:"usageHours,omitempty"`
	// Release is the release that caused the incident, when one did
	// (detail_release.go).
	Release *ReleaseCause `json:"release,omitempty"`
	// Similar is how many other incidents have the same error
	// (similar.go): /api/incidents?status=all&like=<id> lists them.
	Similar int `json:"similar,omitempty"`
	// IncidentRelations are its cause and effect incidents and its
	// blast radius (detail_relations.go).
	IncidentRelations
	// What became of the incident's messages, how it ended and what
	// the page cannot show (responses_ended.go).
	DeliveryOutcome
	IncidentEnd
	// Truncated counts what was left out to keep the response within
	// its size limit; absent when nothing was.
	Truncated *DetailTruncation `json:"truncated,omitempty"`
}

// DetailTruncation counts what a detail response left out, by part.
// Timeline, LogLines and EvidenceLines are the oldest entries;
// EvidenceFacts and EvidenceSections the last ones; Texts is how many
// long strings were shortened (ending in "…").
type DetailTruncation struct {
	Timeline         int `json:"timeline,omitempty"`
	LogLines         int `json:"logLines,omitempty"`
	EvidenceLines    int `json:"evidenceLines,omitempty"`
	EvidenceFacts    int `json:"evidenceFacts,omitempty"`
	EvidenceSections int `json:"evidenceSections,omitempty"`
	Texts            int `json:"texts,omitempty"`
	// Changes, Containers, Deliveries, Affected and Occurrences count
	// the least relevant entries of each list left out, by its bound or
	// by the size limit.
	Changes     int `json:"changes,omitempty"`
	Containers  int `json:"containers,omitempty"`
	Deliveries  int `json:"deliveries,omitempty"`
	Affected    int `json:"affected,omitempty"`
	Occurrences int `json:"occurrences,omitempty"`
	// Warnings, Traffic, Nodes, Probes and WaitReasons count the
	// entries of the insight parts left out the same way.
	Warnings    int `json:"warnings,omitempty"`
	Traffic     int `json:"traffic,omitempty"`
	Nodes       int `json:"nodes,omitempty"`
	Probes      int `json:"probes,omitempty"`
	WaitReasons int `json:"waitReasons,omitempty"`
	// SameTime and UsageHours count the other issues and the usage
	// histories left out.
	SameTime   int `json:"sameTime,omitempty"`
	UsageHours int `json:"usageHours,omitempty"`
	// The incident's own record and its ties: the oldest transitions,
	// break snapshots and phases, the containers of each snapshot,
	// the last steps, related incidents, pagers, sent texts, the
	// earliest moved-in incidents and the changes before recovery.
	Transitions           int `json:"transitions,omitempty"`
	Snapshots             int `json:"snapshots,omitempty"`
	SnapshotRows          int `json:"snapshotRows,omitempty"`
	Phases                int `json:"phases,omitempty"`
	FixSteps              int `json:"fixSteps,omitempty"`
	CausedBy              int `json:"causedBy,omitempty"`
	Causing               int `json:"causing,omitempty"`
	Pager                 int `json:"pager,omitempty"`
	Sent                  int `json:"sent,omitempty"`
	MovedFrom             int `json:"movedFrom,omitempty"`
	ChangedBeforeRecovery int `json:"changedBeforeRecovery,omitempty"`
	// SummaryProof and SummaryTold count the summary's proof items and
	// channels left out; SummaryNext is 1 when its step was.
	SummaryProof int `json:"summaryProof,omitempty"`
	SummaryTold  int `json:"summaryTold,omitempty"`
	SummaryNext  int `json:"summaryNext,omitempty"`
}

// EvidenceSection is what one check read. Status is ok, denied,
// timeout or absent; Reason says why a denied one is (rbac, off).
type EvidenceSection struct {
	Check   string         `json:"check"`
	Title   string         `json:"title"`
	Status  string         `json:"status"`
	Reason  string         `json:"reason,omitempty"`
	Subject string         `json:"subject,omitempty"`
	Run     string         `json:"run,omitempty"`
	Facts   []EvidenceFact `json:"facts"`
	Lines   []string       `json:"lines"`
	Matched []int          `json:"matched"`
	// ReadAt is when the check read this, omitted when unknown.
	ReadAt *time.Time `json:"readAt,omitempty"`
}

// EvidenceFact is one labelled value.
type EvidenceFact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Correlation is the cross-section conclusion.
type Correlation struct {
	Confirmed string         `json:"confirmed,omitempty"`
	Facts     []EvidenceFact `json:"facts"`
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
	// Repeats counts earlier events that said the same and were folded
	// into this one, so a long-running incident keeps its whole story.
	Repeats int `json:"repeats,omitempty"`
}

// StatusResponse is the /api/status response. WatchedNamespaces is ["*"]
// when every namespace is watched. LastScan is the engine's latest loop
// iteration, zero before the first; LastScanLate says it is older than
// kwatch allows. KwatchNamespace and KwatchDeployment name kwatch's own
// Deployment, for the command that reads its logs. Self is kwatch's view
// of itself, absent while no session runs a pipeline, and
// FailedLastHour the message sends that failed in the last hour, absent
// without delivery (responses_self.go).
type StatusResponse struct {
	Version            string          `json:"version"`
	Edition            string          `json:"edition"`
	LastScan           time.Time       `json:"lastScan,omitzero"`
	LastScanLate       bool            `json:"lastScanLate"`
	KwatchNamespace    string          `json:"kwatchNamespace"`
	KwatchDeployment   string          `json:"kwatchDeployment"`
	WatchedNamespaces  []string        `json:"watchedNamespaces"`
	ExcludedNamespaces []string        `json:"excludedNamespaces,omitempty"`
	PerChannelHealth   []ChannelHealth `json:"perChannelHealth"`
	Self               *SelfStatus     `json:"self,omitempty"`
	FailedLastHour     *int            `json:"failedLastHour,omitempty"`
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
