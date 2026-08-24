package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
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
	expanded, err := expandEnv(string(raw))
	if err != nil {
		return err
	}
	if strings.TrimSpace(expanded) == "" {
		return nil
	}
	dec := yaml.NewDecoder(strings.NewReader(expanded))
	dec.KnownFields(true)
	var tmp Config
	return dec.Decode(&tmp)
}

// expandEnv replaces ${VAR} with the environment value (braced-only;
// bare $ is preserved for passwords/hashes). A referenced variable that is
// not set in the environment is reported as an error rather than silently
// expanding to an empty string, which would corrupt the configuration.
var envVarRe = regexp.MustCompile(`\$\{(\w+)\}`)

func expandEnv(s string) (string, error) {
	unset := map[string]bool{}
	out := envVarRe.ReplaceAllStringFunc(s, func(m string) string {
		groups := envVarRe.FindStringSubmatch(m)
		if groups == nil {
			return m
		}
		v, ok := os.LookupEnv(groups[1])
		if !ok {
			unset[groups[1]] = true
			return m
		}
		return v
	})
	if len(unset) > 0 {
		names := make([]string, 0, len(unset))
		for n := range unset {
			names = append(names, n)
		}
		sort.Strings(names)
		return "", fmt.Errorf("environment variable(s) referenced in config are not set: %s", strings.Join(names, ", "))
	}
	return out, nil
}

// LoadConfig loads yaml configuration from file if provided, otherwise
// loads default configuration
// parseConfigFile reads CONFIG_FILE and unmarshals it over a fresh default
// config. A missing or unset file yields the defaults with no error.
func parseConfigFile() (*Config, error) {
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

	expanded, err := expandEnv(string(yamlFile))
	if err != nil {
		klog.ErrorS(err, "failed to expand environment variables in config", "file", configFile)
		return nil, err
	}

	if strings.TrimSpace(expanded) != "" {
		if err = yaml.Unmarshal([]byte(expanded), config); err != nil {
			klog.InfoS("unable to parse config file", "error", err.Error())
			return nil, err
		}
	}

	return config, nil
}

// prepareAllowForbidLists splits namespace and reason lists and validates that
// allow and forbid sides are mutually exclusive.
func prepareAllowForbidLists(config *Config, errs []error) []error {
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

	return errs
}

// prepareConfig normalizes parsed config: splits lists, compiles back-compat
// patterns, consolidates suppression state, and runs full validation.
func prepareConfig(config *Config) []error {
	var errs []error

	errs = prepareAllowForbidLists(config, errs)

	var err error

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

	return append(errs, Validate(config)...)
}

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

func LoadConfig() (*Config, error) {
	config, err := parseConfigFile()
	if err != nil {
		return nil, err
	}

	if errs := prepareConfig(config); len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	warnDeprecatedIgnoreFields(config)

	// SeverityByOwnerKind and SeverityByReason keys must match Kubernetes
	// kinds (e.g. "StatefulSet", "DaemonSet") and event reasons
	// (e.g. "ImagePullBackOff", "Evicted") exactly. The enricher matches
	// case-insensitively, so keys are preserved verbatim here. Do NOT
	// reformat them with strings.Title — that corrupts multi-word kinds
	// like DaemonSet → Daemonset and silently disables user severity config.
	config.SeverityByOwnerKind = cloneMap(config.SeverityByOwnerKind)
	config.SeverityByReason = cloneMap(config.SeverityByReason)

	return config, nil
}

func cloneMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
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
