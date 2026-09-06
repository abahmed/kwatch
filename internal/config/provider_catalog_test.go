package config

import (
	"strings"
	"testing"
)

func TestProviderCatalogChoiceDefaultsAreValid(t *testing.T) {
	for _, field := range ProviderCatalog() {
		if !strings.HasPrefix(field.Validation, "one-of:") {
			continue
		}
		allowed := strings.Split(strings.TrimPrefix(
			field.Validation,
			"one-of:",
		), ",")
		found := false
		for _, value := range allowed {
			if value == field.Default {
				found = true
				break
			}
		}
		if !found {
			t.Errorf(
				"%s.%s default %q is not in %q",
				field.Provider,
				field.Field,
				field.Default,
				allowed,
			)
		}
	}
}

func TestProviderCatalogILertPriority(t *testing.T) {
	for _, field := range ProviderCatalog() {
		if field.Provider != "ilert" || field.Field != "priority" {
			continue
		}
		if field.Type != "string" ||
			field.Validation != "one-of:LOW,HIGH,CRITICAL" ||
			field.Default != "HIGH" {
			t.Fatalf("unexpected iLert priority schema: %#v", field)
		}
		return
	}
	t.Fatal("iLert priority is missing from the provider catalog")
}
