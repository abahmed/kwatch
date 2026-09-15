package config

import "k8s.io/klog/v2"

// warnDeprecatedIgnoreFields logs deprecation warnings for suppression knobs
// consolidated into Silences.
func warnDeprecatedIgnoreFields(config *Config) {
	if len(config.IgnoreContainerNames) > 0 {
		klog.Warning("ignoreContainerNames is deprecated; use silences instead")
	}
	if len(config.IgnoreLogPatterns) > 0 {
		klog.Warning("ignoreLogPatterns is deprecated; use silences instead")
	}
	if len(config.IgnoreContainerMessages) > 0 {
		klog.Warning("ignoreContainerMessages is deprecated; use silences instead")
	}
	if len(config.IgnoreNodeReasons) > 0 {
		klog.Warning("ignoreNodeReasons is deprecated; use silences instead")
	}
	if len(config.IgnoreNodeMessages) > 0 {
		klog.Warning("ignoreNodeMessages is deprecated; use silences instead")
	}
}

// appendIgnoreFieldSilences converts deprecated ignore* config fields into
// synthetic SilenceRules and appends them to the existing silences list.
// This ensures all suppression is consolidated under Silences for unified
// detect-time and post-detect filtering.
func appendIgnoreFieldSilences(c *Config) []SilenceRule {
	var extra []SilenceRule

	if len(c.IgnoreContainerNames) > 0 {
		extra = append(extra, SilenceRule{ContainerNames: c.IgnoreContainerNames})
	}
	if len(c.IgnorePodNames) > 0 {
		extra = append(extra, SilenceRule{PodNamePatterns: c.IgnorePodNames})
	}
	if len(c.IgnoreLogPatterns) > 0 {
		extra = append(extra, SilenceRule{LogPatterns: c.IgnoreLogPatterns})
	}
	if len(c.IgnoreContainerMessages) > 0 {
		extra = append(extra, SilenceRule{
			ContainerMessages: c.IgnoreContainerMessages,
		})
	}
	if len(c.IgnoreNodeReasons) > 0 {
		extra = append(extra, SilenceRule{NodeReasons: c.IgnoreNodeReasons})
	}
	if len(c.IgnoreNodeMessages) > 0 {
		extra = append(extra, SilenceRule{NodeMessages: c.IgnoreNodeMessages})
	}

	return append(c.Silences, extra...)
}
