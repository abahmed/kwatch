package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/abahmed/kwatch/internal/config"
)

const catalogHeader = "# kwatch provider catalog v1"

func main() {
	output := flag.String("output", "", "path to the catalog file to write")
	flag.Parse()
	if *output == "" {
		fmt.Fprintln(os.Stderr, "-output is required")
		os.Exit(2)
	}
	lines, err := catalogLines()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(*output, []byte(content), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func catalogLines() ([]string, error) {
	lines := []string{catalogHeader}
	seen := make(map[string]bool)
	for _, field := range config.ProviderCatalog() {
		if !config.KnownProviders[field.Provider] {
			return nil, fmt.Errorf(
				"provider catalog contains unknown provider %q",
				field.Provider,
			)
		}
		key := field.Provider + "." + field.Field
		if seen[key] {
			return nil, fmt.Errorf("duplicate provider field %q", key)
		}
		if !validProviderFieldType(field.Type) {
			return nil, fmt.Errorf(
				"provider catalog contains unsupported type %q for %s",
				field.Type,
				key,
			)
		}
		seen[key] = true
		lines = append(lines, providerLine(field))
	}
	return lines, nil
}

func validProviderFieldType(fieldType string) bool {
	switch fieldType {
	case "string", "integer", "boolean", "list", "json", "headers":
		return true
	default:
		return false
	}
}

func providerLine(field config.ProviderField) string {
	return strings.Join([]string{
		field.Provider,
		field.DisplayName,
		field.Field,
		field.Type,
		fmt.Sprint(field.Required),
		fmt.Sprint(field.Secret),
		field.Validation,
		field.Default,
		field.Description,
	}, "|")
}
