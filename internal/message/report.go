package message

// Report is a structured, provider-agnostic representation of an incident
// notification. Sections are populated selectively based on the incident's
// reason — nil sections are omitted by renderers.
type Report struct {
	Action    string // "create", "update", "resolved"
	Reason    string
	Severity  string
	Resource  string // "pod", "node", "deployment", etc.
	Name      string
	Namespace string
	Cluster   string

	// Always present
	Summary SummarySection

	// Conditionally populated
	Identity  *IdentitySection
	State     *StateSection
	Diagnosis *DiagnosisSection
	Evidence  *EvidenceSection
	Changes   *ChangesSection
	Runbook   string

	// Type-specific sections (populated only for relevant reasons)
	OOM     *OOMSection
	Probe   *ProbeSection
	Image   *ImageSection
	Pending *PendingSection

	// Node-specific
	SuppressedPods         int
	SuppressedPodSummaries []PodSummaryEntry
}

// SummarySection holds the top-level incident summary.
type SummarySection struct {
	Emoji    string // "🔴", "🟡", "🟢"
	Duration string
	Count    int
	Peak     int
	Label    string // "Out of memory", "Container keeps crashing", etc.
}

// IdentitySection identifies the affected workload/container.
type IdentitySection struct {
	Container string
	Image     string
	Node      string
	OwnerKind string
}

// StateSection holds the current container/incident state.
type StateSection struct {
	Message     string
	ExitCode    int32
	Restarts    int32
	Duration    string
	TotalEvents int
}

// DiagnosisSection holds diagnostic context.
type DiagnosisSection struct {
	Hint    string
	Cause   string
	Impact  string
	Pattern string
}

// EvidenceSection holds logs and events.
type EvidenceSection struct {
	Logs   string
	Events string
}

// ChangesSection holds recent resource changes.
type ChangesSection struct {
	Items         []ChangeItem
	AffectedCount int
}

// ChangeItem is a single recent change entry.
type ChangeItem struct {
	Resource  string
	Reference string
	Type      string
}

// OOMSection holds OOM-specific diagnostics.
type OOMSection struct {
	MemoryLimit string
	Timeline    string
	IsLeak      bool
	LeakCount   int
	WindowMin   int
}

// ProbeSection holds probe-specific diagnostics.
type ProbeSection struct {
	ProbeType string // "liveness", "readiness", "startup"
	Endpoint  string // "HTTP GET /healthz:8080"
}

// ImageSection holds image-pull diagnostics.
type ImageSection struct {
	RegistryHint string
	PullSecrets  bool
}

// PendingSection holds scheduling-specific diagnostics.
type PendingSection struct {
	Delay            string
	ResourceRequests []string
}

// PodSummaryEntry is a lightweight representation of a suppressed pod.
type PodSummaryEntry struct {
	Namespace    string
	PodName      string
	Reason       string
	RestartCount int
}
