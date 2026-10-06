package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"
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
	// CONFIG_FILE is an explicit operator-selected path, not user-controlled input.
	raw, err := os.ReadFile(configFile) // #nosec G304,G703 -- intentional operator path
	if err != nil {
		return err
	}
	if err := validateSecretReferences(string(raw)); err != nil {
		return err
	}
	document, err := expandConfigDocument(string(raw))
	if err != nil || document == nil {
		return err
	}
	// The expanded document must still decode; unknown keys are then
	// reported from the original bytes so line numbers match the file.
	var tmp Config
	if err := document.Decode(&tmp); err != nil {
		return err
	}
	return unknownKeysError(raw)
}

// expandConfigDocument parses the config and then resolves ${VAR} and exact
// ${file:/path} references inside scalar values only. Expanding after parsing
// keeps secret values from changing the YAML structure: a value containing a
// quote, colon, or newline stays one string. Bare $ is preserved for
// passwords and hashes. A referenced variable that is not set is an error
// rather than an empty string that would corrupt the configuration.
//
// Write $${NAME} to keep a literal ${NAME}, for example in a silence regexp
// or a message substring.
var envVarRe = regexp.MustCompile(`(\$?)\$\{(\w+)\}`)
var fileRefRe = regexp.MustCompile(`^\$\{file:(/[^}]*)\}$`)

func expandConfigDocument(raw string) (*yaml.Node, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
		return nil, err
	}
	if document.Kind == 0 {
		return nil, nil
	}
	unset := map[string]bool{}
	if err := expandNode(&document, unset); err != nil {
		return nil, err
	}
	if len(unset) > 0 {
		names := make([]string, 0, len(unset))
		for n := range unset {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, fmt.Errorf(
			"environment variable(s) referenced in config are not set: %s",
			strings.Join(names, ", "),
		)
	}
	return &document, nil
}

func expandNode(node *yaml.Node, unset map[string]bool) error {
	if node.Kind == yaml.ScalarNode {
		if err := expandScalar(node, unset); err != nil {
			return err
		}
	}
	for _, child := range node.Content {
		if err := expandNode(child, unset); err != nil {
			return err
		}
	}
	return nil
}

func expandScalar(node *yaml.Node, unset map[string]bool) error {
	if match := fileRefRe.FindStringSubmatch(node.Value); match != nil {
		// #nosec G304 -- operator-selected config reference
		value, err := os.ReadFile(match[1])
		if err != nil {
			return fmt.Errorf(
				"config file reference %q could not be read: %w",
				match[1], err,
			)
		}
		node.Value = strings.TrimRight(string(value), "\r\n")
		return nil
	}
	if !envVarRe.MatchString(node.Value) {
		return nil
	}
	node.Value = envVarRe.ReplaceAllStringFunc(
		node.Value, func(m string) string {
			parts := envVarRe.FindStringSubmatch(m)
			if parts[1] == "$" {
				return m[1:] // $${NAME} is the literal ${NAME}
			}
			name := parts[2]
			v, ok := os.LookupEnv(name)
			if !ok {
				unset[name] = true
				return m
			}
			return v
		},
	)
	// A plain `port: ${PORT}` must decode as the value's natural type, as it
	// did with text expansion. Quoted scalars stay strings.
	if node.Style == 0 {
		node.Tag = ""
	}
	return nil
}

// parseConfigFile reads CONFIG_FILE and unmarshals it over a fresh default
// config. An unset CONFIG_FILE yields the defaults; a set one that does not
// exist is an error.
func parseConfigFile() (*Config, error) {
	configFile := os.Getenv("CONFIG_FILE")

	config := DefaultConfig()

	if configFile == "" {
		klog.Warning(
			"configuration file not set; using defaults " +
				"(no alert providers)",
		)
		return config, nil
	}

	// CONFIG_FILE is an explicit operator-selected path, not user-controlled input.
	yamlFile, err := os.ReadFile(configFile) // #nosec G304,G703 -- intentional operator path
	if err != nil {
		if os.IsNotExist(err) {
			// CONFIG_FILE names a file the operator meant to use, for
			// example a Secret key. Running on defaults would silently
			// drop every provider, so a missing file stops startup.
			return nil, fmt.Errorf(
				"CONFIG_FILE %q does not exist: check that the mounted "+
					"ConfigMap or Secret has a config.yaml key, or unset "+
					"CONFIG_FILE to run with defaults", configFile)
		}
		klog.InfoS("unable to load config file", "error", err.Error())
		return nil, err
	}
	if err := validateSecretReferences(string(yamlFile)); err != nil {
		klog.ErrorS(err, "unsafe credential in config", "file", configFile)
		return nil, err
	}

	document, err := expandConfigDocument(string(yamlFile))
	if err != nil {
		klog.ErrorS(err, "failed to expand references in config",
			"file", configFile)
		return nil, err
	}
	if document != nil {
		if err = document.Decode(config); err != nil {
			klog.InfoS("unable to parse config file", "error", err.Error())
			return nil, err
		}
		config.unknownKeys = unknownConfigKeys(yamlFile)
	}

	return config, nil
}

// prepareAllowForbidLists splits namespace and reason lists and validates that
// allow and forbid sides are mutually exclusive.
func prepareAllowForbidLists(config *Config, errs []error) []error {
	errs = append(errs, validateNamespaceEntries(config.Namespaces)...)
	errs = append(errs, validateReasonEntries(config.Reasons)...)
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

func validateNamespaceEntries(items []string) []error {
	var errs []error
	for _, item := range items {
		name := strings.TrimPrefix(item, "!")
		if name == "" {
			errs = append(errs, errors.New("namespaces entries must not be empty"))
			continue
		}
		if problems := utilvalidation.IsDNS1123Label(name); len(problems) > 0 {
			errs = append(errs, fmt.Errorf(
				"invalid namespace %q: %s",
				name,
				strings.Join(problems, ", "),
			))
		}
	}
	return errs
}

func validateReasonEntries(items []string) []error {
	var errs []error
	for _, item := range items {
		if strings.TrimSpace(strings.TrimPrefix(item, "!")) == "" {
			errs = append(errs, errors.New("reasons entries must not be empty"))
		}
	}
	return errs
}

// prepareConfig normalizes parsed config: splits lists, consolidates
// suppression into silences, and runs full validation.
func prepareConfig(config *Config) []error {
	var errs []error

	errs = prepareAllowForbidLists(config, errs)

	// Remove synthetic rules from any earlier preparation pass before building
	// them again. Startup CRD overlays may request a second pass.
	if n := config.syntheticSilences; n > 0 && len(config.Silences) >= n {
		config.Silences = config.Silences[:len(config.Silences)-n]
	}
	config.syntheticSilences = 0

	// Consolidation: convert deprecated ignore* fields into synthetic
	// SilenceRules so scope filtering reads one list.
	baseLen := len(config.Silences)
	config.Silences = appendIgnoreFieldSilences(config)
	config.syntheticSilences = len(config.Silences) - baseLen

	return append(errs, Validate(config)...)
}

// LoadConfig reads, overlays and validates the configuration file.
func LoadConfig() (*Config, error) {
	config, err := parseConfigFile()
	if err != nil {
		return nil, err
	}
	if err := applyEnvironmentOverrides(config); err != nil {
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
	config.Runtime = CompileRuntimeConfig(config)

	return config, nil
}

// RebuildAfterOverlay refreshes validation and derived indexes after a
// startup-only configuration source has overlaid the base file.
func RebuildAfterOverlay(c *Config) error {
	if err := applyEnvironmentOverrides(c); err != nil {
		return err
	}
	if errs := prepareConfig(c); len(errs) > 0 {
		return errors.Join(errs...)
	}
	c.Runtime = CompileRuntimeConfig(c)
	return nil
}

// ResetDerivedSilences removes generated ignore* rules before an external
// overlay mutates the serialized Silences field.
func ResetDerivedSilences(c *Config) {
	if n := c.syntheticSilences; n > 0 && len(c.Silences) >= n {
		c.Silences = c.Silences[:len(c.Silences)-n]
	}
	c.syntheticSilences = 0
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
