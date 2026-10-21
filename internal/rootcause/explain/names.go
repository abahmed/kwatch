package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
)

// genericKeys are names too common in failure prose to prove that an
// error names the object: an error about a "host" does not point at the
// object whose key is literally named host, and a ConfigMap called
// "config" is not what "failed to load config" points at. Only a name
// that is exactly one of these is ignored; a compound name such as
// "db-host" is named only as that whole word, so it stays evidence.
// Credential and endpoint keys (password, token, url, port, ...) are
// kept: an authentication or connection error that names one points at
// it.
var genericKeys = map[string]bool{
	"config": true, "configuration": true, "conf": true,
	"value": true, "values": true, "data": true, "name": true,
	"key": true, "keys": true, "default": true, "server": true,
	"host": true, "addr": true, "address": true, "path": true,
	"file": true, "timeout": true, "retry": true, "mode": true,
	"type": true, "image": true, "version": true,
}

// nameWord is name as a lower-case word an error can name: ok is false
// for a name too short to match by anything but accident, or one that is
// a common word.
func nameWord(name string) (string, bool) {
	if len(name) < SpecificityMinName {
		return "", false
	}
	word := strings.ToLower(name)
	return word, !genericKeys[word]
}

// namesOf lists the words that identify id in error text: its name, the
// keys that changed in it and the names of the Services that serve it,
// since a webhook error names the webhook's Service rather than its
// configuration. A name that is a common word is left out.
func (v *view) namesOf(id inventory.EntityID) []string {
	var out []string
	add := func(name string) {
		if word, ok := nameWord(name); ok {
			out = append(out, word)
		}
	}
	add(id.Name)
	for _, key := range v.changedKeys(id) {
		add(key)
	}
	for _, service := range v.s.Model.Related(
		id, inventory.Serves, inventory.Outgoing,
	) {
		add(service.Name)
	}
	return out
}

// mentionsAny reports whether text names one of the lower-case names.
func mentionsAny(text string, names []string) bool {
	text = strings.ToLower(text)
	for _, name := range names {
		if mentionsWord(text, name) {
			return true
		}
	}
	return false
}

// mentionsWord reports name as a whole word of text: not part of a
// longer name. Object names are made of letters, digits and dashes, so
// "api" is not named by "rapid" or "api-gateway" but is by `"api"`,
// "secret api" and "api.example.com".
func mentionsWord(text, name string) bool {
	for from := 0; from < len(text); {
		at := strings.Index(text[from:], name)
		if at < 0 {
			return false
		}
		start := from + at
		end := start + len(name)
		if (start == 0 || !nameByte(text[start-1])) &&
			(end == len(text) || !nameByte(text[end])) {
			return true
		}
		from = start + 1
	}
	return false
}

// nameByte reports a byte that continues an object or key name.
func nameByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' ||
		b == '-' || b == '_'
}

// changedKeys lists the keys of id's ConfigMap or Secret that changed
// ("data.db-password"): an error that quotes one points at the change.
func (v *view) changedKeys(id inventory.EntityID) []string {
	var out []string
	for _, change := range v.changesOf(id) {
		for _, field := range change.Fields {
			if key, ok := strings.CutPrefix(field.Path, "data."); ok {
				out = append(out, key)
			}
		}
	}
	return out
}
