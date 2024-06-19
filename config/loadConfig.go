package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// LoadConfig loads yaml configuration from file if provided, otherwise
// loads default configuration
func LoadConfig() (*Config, error) {
	// initialize configuration
	configFile := os.Getenv("CONFIG_FILE")

	config := DefaultConfig()
	yamlFile, err := os.ReadFile(configFile)
	if err != nil {
		logrus.Warnf("unable to load config file: %s", err.Error())
		return nil, err
	}

	err = yaml.Unmarshal(yamlFile, config)
	if err != nil {
		logrus.Warnf("unable to parse config file: %s", err.Error())
		return nil, err
	}

	// Parse namespace allow/forbid lists
	config.AllowedNamespaces, config.ForbiddenNamespaces =
		getAllowForbidSlices(config.Namespaces)
	if len(config.AllowedNamespaces) > 0 &&
		len(config.ForbiddenNamespaces) > 0 {
		logrus.Error(
			"Either allowed or forbidden namespaces must be set. " +
				"Can't set both")
	}

	// Parse reason allow/forbid lists
	config.AllowedReasons, config.ForbiddenReasons =
		getAllowForbidSlices(config.Reasons)
	if len(config.AllowedReasons) > 0 &&
		len(config.ForbiddenReasons) > 0 {
		logrus.Error("Either allowed or forbidden reasons must be set. " +
			"Can't set both")
	}

	// Prepare ignored pod name patters
	config.IgnorePodNamePatterns, err =
		getCompiledIgnorePatterns(config.IgnorePodNames)
	if err != nil {
		logrus.Errorf("Failed to compile pod name pattern: %s", err.Error())
	}

	// Prepare ignored log patterns
	config.IgnoreLogPatternsCompiled, err =
		getCompiledIgnorePatterns(config.IgnoreLogPatterns)
	if err != nil {
		logrus.Errorf("Failed to compile log pattern: %s", err.Error())
	}

	// Parse proxy config
	if len(config.App.ProxyURL) > 0 {
		os.Setenv("HTTPS_PROXY", config.App.ProxyURL)
	}

	return config, nil
}

// getAllowForbidSlices split input slice into two slices by items start with !
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
