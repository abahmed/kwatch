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
	paths := keyPaths(&document)
	var keys []string
	for _, message := range typeErr.Errors {
		match := unknownFieldRe.FindStringSubmatch(message)
		if match == nil {
			// Type errors such as `port: ${PORT}` before expansion are
			// not unknown keys; the lenient load has already judged them.
			continue
		}
		if len(keys) == maxUnknownKeys {
			keys = append(keys, fmt.Sprintf("%s%d", moreKeys,
				unknownCount(typeErr.Errors)-maxUnknownKeys))
			break
		}
		line, _ := strconv.Atoi(match[1])
		path := paths[lineKey{line, match[2]}]
		if path == "" {
			path = match[2]
		}
		keys = append(keys, fmt.Sprintf("line %d: %s", line, path))
	}
	return keys
}

// maxUnknownKeys bounds the unknown keys reported one by one; the rest
// are counted in one "and N more" entry.
const maxUnknownKeys = 50

// moreKeys starts the entry that counts the unknown keys not listed.
const moreKeys = "and more unknown keys: "

// unknownCount counts the unknown-key messages among errs.
func unknownCount(errs []string) int {
	n := 0
	for _, message := range errs {
		if unknownFieldRe.MatchString(message) {
			n++
		}
	}
	return n
}

// lineKey is a mapping key by its line and name.
type lineKey struct {
	line  int
	field string
}

// keyPaths maps every mapping key of the document to its full dotted
// path, such as "app.clusterNmae" or "silences[0].bogus", in one walk:
// the first key in file order wins a line and name it shares.
func keyPaths(document *yaml.Node) map[lineKey]string {
	out := map[lineKey]string{}
	walkKeys(out, document, "")
	return out
}

// walkKeys records the keys under node, whose path is prefix.
func walkKeys(out map[lineKey]string, node *yaml.Node, prefix string) {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			walkKeys(out, child, prefix)
		}
	case yaml.MappingNode:
		walkMapping(out, node, prefix)
	case yaml.SequenceNode:
		for i, item := range node.Content {
			walkKeys(out, item, fmt.Sprintf("%s[%d]", prefix, i))
		}
	}
}

// walkMapping records one mapping's keys, then the keys of their
// values. Content alternates key and value nodes.
func walkMapping(out map[lineKey]string, node *yaml.Node, prefix string) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		path := key.Value
		if prefix != "" {
			path = prefix + dotted(key.Value)
		}
		if k := (lineKey{key.Line, key.Value}); out[k] == "" {
			out[k] = path
		}
		walkKeys(out, value, path)
	}
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
		if _, n, ok := strings.Cut(key, moreKeys); ok {
			warnings = append(warnings, fmt.Sprintf(
				"%s more config keys are not recognized and are ignored",
				n))
			continue
		}
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
