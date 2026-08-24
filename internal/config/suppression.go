package config

import (
	"regexp"

	"k8s.io/klog/v2"
)

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
	// LogPatterns is an optional list of regex patterns for log content to silence.
	LogPatterns []string `yaml:"logPatterns"`
	// ContainerMessages is an optional list of substrings; if a container
	// status message contains any entry, the incident is suppressed.
	ContainerMessages []string `yaml:"containerMessages"`
	// NodeReasons is an optional list of node reasons to silence.
	NodeReasons []string `yaml:"nodeReasons"`
	// NodeMessages is an optional list of substrings; if a node condition
	// message contains any entry, the incident is suppressed.
	NodeMessages []string `yaml:"nodeMessages"`
}

// SuppressionIndex is a flat compiled view of all suppression rules (both from
// explicit Silences and deprecated ignore* fields) for efficient detect-time
// filtering.
type SuppressionIndex struct {
	ContainerNames    []string
	PodNamePatterns   []*regexp.Regexp
	LogPatterns       []*regexp.Regexp
	ContainerMessages []string
	NodeReasons       []string
	NodeMessages      []string
}

// suppressionBuilder accumulates deduplicated suppression entries from both
// SilenceRules and the deprecated ignore* config fields.
type suppressionBuilder struct {
	idx          SuppressionIndex
	containers   map[string]bool
	podPatterns  map[string]bool
	logPatterns  map[string]bool
	messages     map[string]bool
	nodeReasons  map[string]bool
	nodeMessages map[string]bool
}

func newSuppressionBuilder() *suppressionBuilder {
	return &suppressionBuilder{
		containers:   map[string]bool{},
		podPatterns:  map[string]bool{},
		logPatterns:  map[string]bool{},
		messages:     map[string]bool{},
		nodeReasons:  map[string]bool{},
		nodeMessages: map[string]bool{},
	}
}

func (b *suppressionBuilder) addContainers(items []string) {
	for _, n := range items {
		if !b.containers[n] {
			b.idx.ContainerNames = append(b.idx.ContainerNames, n)
			b.containers[n] = true
		}
	}
}

func (b *suppressionBuilder) addPodPatterns(patterns []string, invalidMsg string) {
	for _, p := range patterns {
		if b.podPatterns[p] {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			klog.ErrorS(err, invalidMsg, "pattern", p)
			continue
		}
		b.idx.PodNamePatterns = append(b.idx.PodNamePatterns, re)
		b.podPatterns[p] = true
	}
}

func (b *suppressionBuilder) addLogPatterns(patterns []string, invalidMsg string) {
	for _, p := range patterns {
		if b.logPatterns[p] {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			klog.ErrorS(err, invalidMsg, "pattern", p)
			continue
		}
		b.idx.LogPatterns = append(b.idx.LogPatterns, re)
		b.logPatterns[p] = true
	}
}

func (b *suppressionBuilder) addMessages(items []string) {
	for _, m := range items {
		if !b.messages[m] {
			b.idx.ContainerMessages = append(b.idx.ContainerMessages, m)
			b.messages[m] = true
		}
	}
}

func (b *suppressionBuilder) addNodeReasons(items []string) {
	for _, r := range items {
		if !b.nodeReasons[r] {
			b.idx.NodeReasons = append(b.idx.NodeReasons, r)
			b.nodeReasons[r] = true
		}
	}
}

func (b *suppressionBuilder) addNodeMessages(items []string) {
	for _, m := range items {
		if !b.nodeMessages[m] {
			b.idx.NodeMessages = append(b.idx.NodeMessages, m)
			b.nodeMessages[m] = true
		}
	}
}

func (b *suppressionBuilder) addRule(sr SilenceRule) {
	b.addContainers(sr.ContainerNames)
	b.addPodPatterns(sr.PodNamePatterns, "invalid suppression pod name pattern")
	b.addLogPatterns(sr.LogPatterns, "invalid suppression log pattern")
	b.addMessages(sr.ContainerMessages)
	b.addNodeReasons(sr.NodeReasons)
	b.addNodeMessages(sr.NodeMessages)
}

// BuildSuppressionIndex merges deprecated ignore* fields with explicit
// SilenceRules and returns a flat SuppressionIndex for detect-time filters.
func (c *Config) BuildSuppressionIndex() SuppressionIndex {
	b := newSuppressionBuilder()

	for _, sr := range c.Silences {
		b.addRule(sr)
	}
	// Also include deprecated ignore* fields directly (they may also appear as
	// synthetic SilenceRules, but this ensures they're present regardless).
	b.addContainers(c.IgnoreContainerNames)
	// Compile these if not already covered by silences
	b.addPodPatterns(c.IgnorePodNames, "invalid ignorePodName pattern")
	b.addLogPatterns(c.IgnoreLogPatterns, "invalid ignoreLogPattern")
	b.addMessages(c.IgnoreContainerMessages)
	b.addNodeReasons(c.IgnoreNodeReasons)
	b.addNodeMessages(c.IgnoreNodeMessages)
	return b.idx
}
