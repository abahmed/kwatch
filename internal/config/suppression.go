package config

// SilenceRule defines an alert suppression rule.
// An incident matching any silence rule is suppressed entirely.
type SilenceRule struct {
	// Namespaces is an optional list of namespaces to silence.
	Namespaces []string `yaml:"namespaces"`
	// Reasons is an optional list of reasons to silence.
	Reasons []string `yaml:"reasons"`
	// PodNamePatterns is an optional list of regex patterns for pod names to silence.
	PodNamePatterns []string `yaml:"podNamePatterns"`
	// ContainerNames is an optional list of container names to silence.
	ContainerNames []string `yaml:"containerNames"`
	// ContainerMessages is an optional list of substrings; if a container
	// status message contains any entry, the incident is suppressed.
	ContainerMessages []string `yaml:"containerMessages"`
	// EventMessages is an optional list of substrings; if an Event message
	// attached to the affected Pod contains any entry, the incident is
	// suppressed.
	EventMessages []string `yaml:"eventMessages"`
	// NodeReasons is an optional list of node reasons to silence.
	NodeReasons []string `yaml:"nodeReasons"`
	// NodeMessages is an optional list of substrings; if a node condition
	// message contains any entry, the incident is suppressed.
	NodeMessages []string `yaml:"nodeMessages"`
}

// IsEmpty reports whether the rule sets no matching field. An empty rule
// would match every finding, so validation rejects it and scope matching
// treats it as matching nothing.
func (r SilenceRule) IsEmpty() bool {
	return len(r.Namespaces) == 0 && len(r.Reasons) == 0 &&
		len(r.PodNamePatterns) == 0 && len(r.ContainerNames) == 0 &&
		len(r.ContainerMessages) == 0 && len(r.EventMessages) == 0 &&
		len(r.NodeReasons) == 0 && len(r.NodeMessages) == 0
}
