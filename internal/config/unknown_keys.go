package config

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// unknownFieldRe matches the yaml.v3 strict-decoding message for a key no
// struct field reads, for example
// "line 4: field clusterNmae not found in type config.App".
var unknownFieldRe = regexp.MustCompile(
	`^line (\d+): field (.+) not found in type `)

// unknownConfigKeys strictly decodes the original file bytes and returns
// one "line N: full.key.path" entry per key no Config field reads, in file
// order. The original bytes are decoded, not the expanded document, so the
// line numbers match the file the operator edits. Loading never fails
// here: an old key from a previous release must not stop an upgrade.
func unknownConfigKeys(raw []byte) []string {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	var strict Config
	err := decoder.Decode(&strict)
	var typeErr *yaml.TypeError
	if !errors.As(err, &typeErr) {
		return nil
	}
	var document yaml.Node
	if yaml.Unmarshal(raw, &document) != nil {
		return nil
	}
	var keys []string
	for _, message := range typeErr.Errors {
		match := unknownFieldRe.FindStringSubmatch(message)
		if match == nil {
			// Type errors such as `port: ${PORT}` before expansion are
			// not unknown keys; the lenient load has already judged them.
			continue
		}
		line, _ := strconv.Atoi(match[1])
		path := keyPath(&document, line, match[2])
		if path == "" {
			path = match[2]
		}
		keys = append(keys, fmt.Sprintf("line %d: %s", line, path))
	}
	return keys
}

// keyPath finds the mapping key named field on line and returns its full
// dotted path, such as "app.clusterNmae" or "silences[0].bogus". It
// returns "" when no such key exists.
func keyPath(node *yaml.Node, line int, field string) string {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if path := keyPath(child, line, field); path != "" {
				return path
			}
		}
	case yaml.MappingNode:
		return mappingKeyPath(node, line, field)
	case yaml.SequenceNode:
		for i, item := range node.Content {
			if path := keyPath(item, line, field); path != "" {
				return fmt.Sprintf("[%d]%s", i, dotted(path))
			}
		}
	}
	return ""
}

// mappingKeyPath searches the keys of one mapping node and then its values.
// Content alternates key and value nodes.
func mappingKeyPath(node *yaml.Node, line int, field string) string {
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Line == line && key.Value == field {
			return key.Value
		}
		if path := keyPath(value, line, field); path != "" {
			return key.Value + dotted(path)
		}
	}
	return ""
}

// dotted joins a child path to its parent: a sequence index attaches
// directly ("[0]"), a key needs a dot (".bogus").
func dotted(child string) string {
	if strings.HasPrefix(child, "[") {
		return child
	}
	return "." + child
}

// unknownKeysError reports unknown keys as one strict-lint error, or nil.
func unknownKeysError(raw []byte) error {
	keys := unknownConfigKeys(raw)
	if len(keys) == 0 {
		return nil
	}
	return fmt.Errorf("config keys are not recognized: %s",
		strings.Join(keys, "; "))
}

// unknownKeyWarnings reports each unknown key once, in file order.
func unknownKeyWarnings(cfg *Config) []string {
	warnings := make([]string, 0, len(cfg.unknownKeys))
	for _, key := range cfg.unknownKeys {
		warnings = append(warnings, fmt.Sprintf(
			"config key is not recognized and is ignored (%s); it may be "+
				"misspelled or removed in this release", key))
	}
	return warnings
}

// NoteOverlayUnknownKeys records the keys an overlay document, such as a
// KwatchConfig spec, sets that no field reads. Each entry is prefixed with
// source so Warnings tells the operator which document holds the typo.
func NoteOverlayUnknownKeys(c *Config, source string, raw []byte) {
	for _, key := range unknownConfigKeys(raw) {
		c.unknownKeys = append(c.unknownKeys, source+" "+key)
	}
}
