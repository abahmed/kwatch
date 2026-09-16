package config

import (
	"regexp"
)

// RuntimeConfigFor returns the derived snapshot used by runtime components.
// Loaded configurations already carry it; direct callers are compiled without
// mutating the YAML-facing configuration.
func RuntimeConfigFor(c *Config) RuntimeConfig {
	if c == nil {
		return RuntimeConfig{}
	}
	if c.Runtime.Compiled() {
		return c.Runtime
	}
	return CompileRuntimeConfig(c)
}

// Compiled reports whether this snapshot was produced from a configuration.
// It lets package-level tests that build Config values directly retain the
// same defaults as the file loader without making runtime consumers read raw
// configuration fields.
func (r RuntimeConfig) Compiled() bool {
	return r.compiled
}

// Application returns the normalized application identity and outbound
// settings. The value contains no mutable reference fields.
func (r RuntimeConfig) Application() ApplicationRuntime {
	return r.application
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func cloneSuppressionIndex(index SuppressionIndex) SuppressionIndex {
	return SuppressionIndex{
		ContainerNames:    cloneStrings(index.ContainerNames),
		PodNamePatterns:   append([]*regexp.Regexp(nil), index.PodNamePatterns...),
		LogPatterns:       append([]*regexp.Regexp(nil), index.LogPatterns...),
		ContainerMessages: cloneStrings(index.ContainerMessages),
		EventMessages:     cloneStrings(index.EventMessages),
		NodeReasons:       cloneStrings(index.NodeReasons),
		NodeMessages:      cloneStrings(index.NodeMessages),
	}
}

func cloneActiveProbeMonitor(m ActiveProbeMonitor) ActiveProbeMonitor {
	m.HTTP = append([]HTTPProbeTarget(nil), m.HTTP...)
	m.TCP = append([]TCPProbeTarget(nil), m.TCP...)
	m.DNS = append([]DNSProbeTarget(nil), m.DNS...)
	m.ExcludeNamespaces = cloneStrings(m.ExcludeNamespaces)
	return m
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneSilenceRules(rules []SilenceRule) []SilenceRule {
	if len(rules) == 0 {
		return nil
	}
	result := make([]SilenceRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, SilenceRule{
			Namespaces:        cloneStrings(rule.Namespaces),
			Reasons:           cloneStrings(rule.Reasons),
			PodNamePatterns:   cloneStrings(rule.PodNamePatterns),
			ContainerNames:    cloneStrings(rule.ContainerNames),
			LogPatterns:       cloneStrings(rule.LogPatterns),
			ContainerMessages: cloneStrings(rule.ContainerMessages),
			EventMessages:     cloneStrings(rule.EventMessages),
			NodeReasons:       cloneStrings(rule.NodeReasons),
			NodeMessages:      cloneStrings(rule.NodeMessages),
		})
	}
	return result
}
