package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/abahmed/kwatch/internal/config"
)

const catalogHeader = "# kwatch config catalog v1"

var managerHiddenPaths = map[string]bool{
	"alert": true,
}

type metadata struct {
	category    string
	description string
	status      string
	replacement string
}

func main() {
	output := flag.String("output", "", "path to the catalog file to write")
	flag.Parse()
	if *output == "" {
		fmt.Fprintln(os.Stderr, "-output is required")
		os.Exit(2)
	}

	old, oldOrder := readMetadata("deploy/config-catalog.tsv")
	friendly := readFriendlyMetadata("deploy/config-catalog.meta.tsv")
	seen := make(map[string]bool)
	var discovered []string
	var missing []string
	walk(reflect.TypeOf(config.Config{}), reflect.ValueOf(config.DefaultConfig()).Elem(), "", old, friendly, &seen, &discovered, &missing)
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "missing friendly metadata for config field(s): %s\n", strings.Join(missing, ", "))
		os.Exit(1)
	}

	lines := []string{catalogHeader}
	for _, line := range oldOrder {
		path := strings.SplitN(line, "|", 2)[0]
		if seen[path] {
			lines = append(lines, line)
			delete(seen, path)
		}
	}
	for _, line := range discovered {
		path := strings.SplitN(line, "|", 2)[0]
		if seen[path] {
			lines = append(lines, line)
			delete(seen, path)
		}
	}

	if err := os.WriteFile(*output, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func readMetadata(path string) (map[string]string, []string) {
	result := make(map[string]string)
	var order []string
	raw, err := os.ReadFile(path)
	if err != nil {
		return result, order
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) == 2 {
			result[parts[0]] = line
			order = append(order, line)
		}
	}
	return result, order
}

func readFriendlyMetadata(path string) map[string]metadata {
	result := make(map[string]metadata)
	raw, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) == 5 {
			result[parts[0]] = metadata{category: parts[1], description: parts[2], status: parts[3], replacement: parts[4]}
		}
	}
	return result
}

func walk(t reflect.Type, value reflect.Value, prefix string, old map[string]string, friendly map[string]metadata, seen *map[string]bool, discovered *[]string, missing *[]string) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
		if value.IsValid() && !value.IsNil() {
			value = value.Elem()
		}
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		if managerHiddenPaths[path] {
			continue
		}
		*seen = addExisting(path, old, *seen)
		fieldValue := value.Field(i)
		fieldType := field.Type
		if fieldType.Kind() == reflect.Struct {
			if _, ok := old[path]; ok {
				continue
			}
			walk(fieldType, fieldValue, path, old, friendly, seen, discovered, missing)
			continue
		}
		if _, ok := old[path]; ok {
			continue
		}
		meta, ok := friendly[path]
		if !ok {
			*missing = append(*missing, path)
			continue
		}
		*discovered = append(*discovered, catalogLine(path, fieldType, fieldValue, meta))
		// Mark newly discovered fields so the second output pass includes them.
		// Without this, adding a config field silently dropped it from the
		// generated catalog even though metadata validation succeeded.
		(*seen)[path] = true
	}
}

func addExisting(path string, old map[string]string, seen map[string]bool) map[string]bool {
	if _, ok := old[path]; ok {
		seen[path] = true
	}
	return seen
}

func catalogLine(path string, t reflect.Type, value reflect.Value, meta metadata) string {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	typeName := "string"
	switch t.Kind() {
	case reflect.Bool:
		typeName = "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		typeName = "integer"
	case reflect.Float32, reflect.Float64:
		typeName = "float"
	case reflect.Slice, reflect.Array:
		typeName = "list"
	case reflect.Map:
		typeName = "json"
	}
	defaultValue := "empty"
	if value.IsValid() && !(value.Kind() == reflect.Pointer && value.IsNil()) {
		var raw []byte
		if typeName == "list" || typeName == "json" {
			raw, _ = json.Marshal(value.Interface())
		} else {
			raw = []byte(fmt.Sprint(value.Interface()))
		}
		defaultValue = strings.ReplaceAll(string(raw), "|", "/")
		if defaultValue == "" {
			defaultValue = "empty"
		}
	}
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s", path, typeName, defaultValue, meta.category, meta.description, meta.status, meta.replacement)
}
