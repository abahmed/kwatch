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
			klog.Warning("config file not found; using default (no alert providers)", "path", configFile)
			return config, nil
		}
		klog.InfoS("unable to load config file", "error", err.Error())
		return nil, err
	}

	// B1: env-var interpolation — ${VAR} is replaced from environment
	expanded := os.Expand(string(yamlFile), func(k string) string { return os.Getenv(k) })

	err = yaml.Unmarshal([]byte(expanded), config)
	if err != nil {
		klog.InfoS("unable to parse config file", "error", err.Error())
		return nil, err
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

	// Prepare ignored pod name patters
	config.IgnorePodNamePatterns, err =
		getCompiledIgnorePatterns(config.IgnorePodNames)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to compile pod name pattern: %w", err))
	}

	// Prepare ignored log patterns
	config.IgnoreLogPatternsCompiled, err =
		getCompiledIgnorePatterns(config.IgnoreLogPatterns)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to compile log pattern: %w", err))
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
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
