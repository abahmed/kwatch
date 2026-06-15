package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
)

// LintStrict re-decodes the config file with KnownFields(true) to reject
// unknown keys, catching typos and removed fields. Used by kwatch lint --strict.
// Runtime LoadConfig stays lenient for back-compat.
func LintStrict() error {
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		return nil
	}
	raw, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}
	expanded := expandEnv(string(raw))
	if strings.TrimSpace(expanded) == "" {
		return nil
	}
	dec := yaml.NewDecoder(strings.NewReader(expanded))
	dec.KnownFields(true)
	var tmp Config
	return dec.Decode(&tmp)
}

// expandEnv replaces ${VAR} with the environment value (braced-only;
// bare $ is preserved for passwords/hashes).
var envVarRe = regexp.MustCompile(`\$\{(\w+)\}`)

func expandEnv(s string) string {
	return envVarRe.ReplaceAllStringFunc(s, func(m string) string {
		groups := envVarRe.FindStringSubmatch(m)
		if groups == nil {
			return m
		}
		return os.Getenv(groups[1])
	})
}

// LoadConfig loads yaml configuration from file if provided, otherwise
// loads default configuration
func LoadConfig() (*Config, error) {
	configFile := os.Getenv("CONFIG_FILE")

	config := DefaultConfig()

	if configFile == "" {
		klog.Warning("no CONFIG_FILE set; using default (no alert providers)")
		return config, nil
	}

	yamlFile, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			klog.InfoS("config file not found; using default (no alert providers)", "path", configFile)
			return config, nil
		}
		klog.InfoS("unable to load config file", "error", err.Error())
		return nil, err
	}

	expanded := expandEnv(string(yamlFile))

	if strings.TrimSpace(expanded) != "" {
		if err = yaml.Unmarshal([]byte(expanded), config); err != nil {
			klog.InfoS("unable to parse config file", "error", err.Error())
			return nil, err
		}
	}

	var errs []error

	// Parse namespace allow/forbid lists
	config.AllowedNamespaces, config.ForbiddenNamespaces =
		getAllowForbidSlices(config.Namespaces)
	if len(config.AllowedNamespaces) > 0 &&
		len(config.ForbiddenNamespaces) > 0 {
		errs = append(errs,
			errors.New("either allowed or forbidden namespaces must be set, can't set both"))
	}
	if config.NamespaceSelector != "" && len(config.Namespaces) > 0 {
		errs = append(errs,
			errors.New("namespaceSelector and namespaces are mutually exclusive"))
	}

	// Parse reason allow/forbid lists
	config.AllowedReasons, config.ForbiddenReasons =
		getAllowForbidSlices(config.Reasons)
	if len(config.AllowedReasons) > 0 &&
		len(config.ForbiddenReasons) > 0 {
		errs = append(errs,
			errors.New("either allowed or forbidden reasons must be set, can't set both"))
	}

	// Prepare ignored pod name patters (compiled for back-compat)
	config.IgnorePodNamePatterns, err =
		getCompiledIgnorePatterns(config.IgnorePodNames)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to compile pod name pattern: %w", err))
	}

	// Prepare ignored log patterns (compiled for back-compat)
	config.IgnoreLogPatternsCompiled, err =
		getCompiledIgnorePatterns(config.IgnoreLogPatterns)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to compile log pattern: %w", err))
	}

	// Consolidation: convert deprecated ignore* fields into synthetic
	// SilenceRules so detect-time and post-detect filters both read from
	// the unified Silences / SuppressionIndex.
	config.Silences = appendIgnoreFieldSilences(config)

	// Build suppression index for detect-time filters
	config.Suppression = config.BuildSuppressionIndex()

	errs = append(errs, Validate(config)...)

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	// Deprecation warnings for suppression knobs being consolidated into Silences
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

	return config, nil
}

func getAllowForbidSlices(items []string) (allow []string, forbid []string) {
	allow = make([]string, 0)
	forbid = make([]string, 0)
	for _, item := range items {
		if clean := strings.TrimPrefix(item, "!"); item != clean {
			forbid = append(forbid, clean)
			continue
		}
		allow = append(allow, item)
	}
	return allow, forbid
}

func getCompiledIgnorePatterns(patterns []string) (compiledPatterns []*regexp.Regexp, err error) {
	compiledPatterns = make([]*regexp.Regexp, 0)

	for _, pattern := range patterns {
		compiledPattern, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to compile pattern '%s'", pattern)
		}
		compiledPatterns = append(compiledPatterns, compiledPattern)
	}

	return compiledPatterns, nil
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
		extra = append(extra, SilenceRule{ContainerMessages: c.IgnoreContainerMessages})
	}
	if len(c.IgnoreNodeReasons) > 0 {
		extra = append(extra, SilenceRule{NodeReasons: c.IgnoreNodeReasons})
	}
	if len(c.IgnoreNodeMessages) > 0 {
		extra = append(extra, SilenceRule{NodeMessages: c.IgnoreNodeMessages})
	}

	return append(c.Silences, extra...)
}
