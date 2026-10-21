package config

import (
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// cleanLicenseKey removes every whitespace and control character from a
// license key. A key pasted from an email or wrapped by a Secret often
// carries a stray space or newline, and the key's own alphabet has none, so
// removing them never changes a valid key. A key that is still not valid is
// reported by the license manager (state "invalid"), never by a config
// error: a license typo must not stop monitoring.
func cleanLicenseKey(key string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return -1
		}
		return r
	}, key)
}

// literalLicenseKeyWarning is the warning for a license key written as
// text in the configuration file. The file is usually a ConfigMap, which
// anyone who can read ConfigMaps can read; the key is a credential. Env
// expansion and ${file:/path} references keep it out of the file.
const literalLicenseKeyWarning = "license.key is written as text in the " +
	"configuration; keep the key out of a ConfigMap and set " +
	"KWATCH_LICENSE_KEY from a Secret (the chart does) or use a " +
	"${file:/path} reference"

// fileWarnings reports how the raw configuration file was written, which
// is lost once ${...} references are expanded.
func fileWarnings(raw []byte) []string {
	var document yaml.Node
	if yaml.Unmarshal(raw, &document) != nil {
		return nil
	}
	root := yamlDocumentRoot(&document)
	key := yamlMapValue(yamlMapValue(root, "license"), "key")
	if key == nil || key.Kind != yaml.ScalarNode || key.Value == "" ||
		strings.Contains(key.Value, "${") {
		return nil
	}
	return []string{literalLicenseKeyWarning}
}
