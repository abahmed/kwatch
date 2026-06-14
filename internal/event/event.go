package event

// Event used to represent info needed by providers to send messages
type Event struct {
	Resource      string            // "pod", "node", "pvc"
	PodName       string
	ContainerName string
	Namespace     string
	NodeName      string
	Reason        string
	Events        string
	Logs          string
	Labels        map[string]string
	OwnerKind     string
	RestartCount  int
	Hint          string // Pre-computed diagnostic hint; empty = auto-generate from Reason
	Severity      string // Override severity; empty = let enricher decide from OwnerKind
	IncludeEvents bool   // If false, omit events section from output
	IncludeLogs   bool   // If false, omit logs section from output
}
