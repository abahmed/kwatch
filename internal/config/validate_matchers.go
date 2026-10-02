package config

import (
	"fmt"
	"strings"
)

// validateEmptyMatchers rejects blank entries in silence and ignore lists.
// A blank regular expression or substring matches every finding, so one
// stray "" in a list would silently suppress everything it applies to.
func validateEmptyMatchers(cfg *Config) []error {
	var errs []error
	for i, rule := range userSilences(cfg) {
		for _, field := range silenceMatcherFields(rule) {
			errs = append(errs, blankEntryErrors(
				fmt.Sprintf("silences[%d].%s", i, field.name),
				field.values)...)
		}
	}
	for _, field := range ignoreMatcherFields(cfg) {
		errs = append(errs, blankEntryErrors(field.name, field.values)...)
	}
	return errs
}

type matcherField struct {
	name   string
	values []string
}

// userSilences returns the silences written by the operator. Validation
// runs after the deprecated ignore* fields became synthetic rules at the
// end of the list; those are checked under their own field names.
func userSilences(cfg *Config) []SilenceRule {
	n := len(cfg.Silences) - cfg.syntheticSilences
	if n < 0 {
		n = 0
	}
	return cfg.Silences[:n]
}

func silenceMatcherFields(rule SilenceRule) []matcherField {
	return []matcherField{
		{"namespaces", rule.Namespaces},
		{"reasons", rule.Reasons},
		{"podNamePatterns", rule.PodNamePatterns},
		{"containerNames", rule.ContainerNames},
		{"containerMessages", rule.ContainerMessages},
		{"eventMessages", rule.EventMessages},
		{"nodeReasons", rule.NodeReasons},
		{"nodeMessages", rule.NodeMessages},
	}
}

func ignoreMatcherFields(cfg *Config) []matcherField {
	return []matcherField{
		{"ignoreContainerNames", cfg.IgnoreContainerNames},
		{"ignorePodNames", cfg.IgnorePodNames},
		{"ignoreContainerMessages", cfg.IgnoreContainerMessages},
		{"ignoreNodeReasons", cfg.IgnoreNodeReasons},
		{"ignoreNodeMessages", cfg.IgnoreNodeMessages},
	}
}

func blankEntryErrors(field string, values []string) []error {
	var errs []error
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			errs = append(errs, fmt.Errorf(
				"%s[%d] must not be empty: a blank entry matches "+
					"everything", field, i))
		}
	}
	return errs
}
