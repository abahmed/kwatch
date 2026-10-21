package config

import (
	"reflect"
	"strings"
)

// KnownKey reports whether path (for example "license", "key") names a
// setting some Config field reads. A path that runs into a map (the alert
// providers, templates, runbooks) is known below it, and so is a path
// through a list. The KwatchConfig guard uses it to tell a setting it must
// refuse from a typo, which is only warned about.
func KnownKey(path ...string) bool {
	return knownIn(reflect.TypeOf(Config{}), path)
}

func knownIn(t reflect.Type, path []string) bool {
	if len(path) == 0 {
		return true
	}
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Map:
		return true
	case reflect.Struct:
	default:
		return false
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		if name == "" || name == "-" || name != path[0] {
			continue
		}
		return knownIn(field.Type, path[1:])
	}
	return false
}
