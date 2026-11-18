package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/format"
)

// ChangeItem is one change people made to the incident's root or an
// object near it (what it uses, routes to it, its node). When is
// "before" for a change made before the failure began and "during"
// after. Actor is the tool that wrote it (a field
// manager), App the GitOps app that owns it, Note the change-cause it
// was recorded with; each is empty when unknown.
type ChangeItem struct {
	At        time.Time `json:"at"`
	When      string    `json:"when"`
	Kind      string    `json:"kind"`
	Namespace string    `json:"namespace,omitempty"`
	Name      string    `json:"name"`
	Summary   string    `json:"summary"`
	Actor     string    `json:"actor,omitempty"`
	App       string    `json:"app,omitempty"`
	Note      string    `json:"note,omitempty"`
	// Fixing marks the change kwatch judged most likely fixed it.
	Fixing bool `json:"fixing,omitempty"`
	// Cause marks the change the likely cause blames, so the page can
	// name it rather than the nearest change.
	Cause bool `json:"cause,omitempty"`
	// Why says in plain words how the changed object is linked to the
	// failing objects; empty when no link is known.
	Why string `json:"why,omitempty"`
	// Fields are the changed fields with their values before and
	// after, for a diff; at most six (see incident.SafeChange).
	Fields []FieldDiff `json:"fields,omitempty"`
	// Links lead to where the change came from: its GitOps app, the
	// commit or repository and the image page (source_links.go). A
	// link without a URL is shown as text.
	Links []format.SourceLink `json:"links,omitempty"`
	// source is what the change records of its origin, for Links.
	source format.ChangeSource
}

// SeenBefore is what kwatch remembers of earlier occurrences of the
// same failure on the same root. InWindow counts those that started in
// the last WindowDays days; Capped says kwatch keeps only the newest
// few, so the real count may be higher. Recent is newest first.
type SeenBefore struct {
	InWindow   int              `json:"inWindow"`
	WindowDays int              `json:"windowDays"`
	Capped     bool             `json:"capped,omitempty"`
	PreviousID string           `json:"previousId,omitempty"`
	Recent     []PastOccurrence `json:"recent"`
}

// PastOccurrence is one earlier occurrence: when it started and ended
// and how it ended (rollback, config fix, node replaced, change,
// recovered without change), empty when unknown.
type PastOccurrence struct {
	Opened   time.Time `json:"opened"`
	Resolved time.Time `json:"resolved,omitzero"`
	Fix      string    `json:"fix,omitempty"`
	Mode     string    `json:"mode,omitempty"`
}

// ContainerRow is one container of the affected pods as kwatch sees it
// now. Usage fields are set only when kwatch reads kubelet usage (Pro);
// UsageAt is when it last read them. Bytes and millicores, 0 when
// unknown.
type ContainerRow struct {
	Namespace    string    `json:"namespace"`
	Pod          string    `json:"pod"`
	Container    string    `json:"container"`
	Ready        *bool     `json:"ready,omitempty"`
	Restarts     int       `json:"restarts"`
	State        string    `json:"state,omitempty"`
	StateReason  string    `json:"stateReason,omitempty"`
	LastReason   string    `json:"lastReason,omitempty"`
	LastExitCode *int      `json:"lastExitCode,omitempty"`
	LastFinished time.Time `json:"lastFinished,omitzero"`
	Image        string    `json:"image,omitempty"`
	MemoryUsed   int64     `json:"memoryUsed,omitempty"`
	MemoryReq    int64     `json:"memoryRequest,omitempty"`
	MemoryLimit  int64     `json:"memoryLimit,omitempty"`
	CPUUsed      int64     `json:"cpuUsedMilli,omitempty"`
	CPUReq       int64     `json:"cpuRequestMilli,omitempty"`
	CPULimit     int64     `json:"cpuLimitMilli,omitempty"`
	UsageAt      time.Time `json:"usageAt,omitzero"`
}

// DeliveryAttempt is what became of one message about the incident at
// one alert channel. Channel is empty when no channel received it;
// Result is ok, failed, retrying, carried, not routed or paging only;
// Reason says in a few plain words why a send failed ("token refused
// (401)"), absent when it worked.
type DeliveryAttempt struct {
	At        time.Time `json:"at"`
	Channel   string    `json:"channel,omitempty"`
	Name      string    `json:"name,omitempty"`
	Kind      string    `json:"kind"`
	Placement string    `json:"placement,omitempty"`
	Result    string    `json:"result"`
	Retries   int       `json:"retries,omitempty"`
	Carrier   string    `json:"carrier,omitempty"`
	Fallback  string    `json:"fallbackFor,omitempty"`
	Error     string    `json:"error,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	// Keyed marks a send to a provider that holds alerts by key (a
	// pager, an issue tracker or a keyed webhook). It does not say the
	// provider pages a person; channelKind does.
	Keyed bool `json:"pager,omitempty"`
}

// AffectedObject is one object the incident is about. Status is
// failing, cause (the object the likely cause blames, when it does not
// fail itself), affected (hit by the failure, not failing itself),
// recovered (the incident resolved) or gone (deleted). Since is when it started
// failing, omitted when unknown. Commands are ready to copy.
type AffectedObject struct {
	Kind      string    `json:"kind"`
	Namespace string    `json:"namespace,omitempty"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Root      bool      `json:"root,omitempty"`
	Since     time.Time `json:"since,omitzero"`
	Commands  []string  `json:"commands"`
}

// Phase is one stretch of the incident's states over time: failing,
// recovering (looked better, kwatch waiting to be sure) or quiet
// (resolved before it came back). To is omitted on the last phase of
// an incident still open: it lasts until now.
type Phase struct {
	State string    `json:"state"`
	From  time.Time `json:"from"`
	To    time.Time `json:"to,omitzero"`
	// Began, on the first phase only, is when the incident really began
	// when the phase window cut that start off: From is then the start
	// of the window and the time before it is not drawn.
	Began time.Time `json:"began,omitzero"`
}
