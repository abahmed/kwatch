package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ProviderField describes one field supported by the guided installer.
// Type is one of string, integer, boolean, list, json, or headers.
type ProviderField struct {
	Provider    string
	DisplayName string
	Field       string
	Type        string
	Required    bool
	Secret      bool
	Validation  string
	Default     string
	Description string
	Group       string
	Condition   string
}

var providerCatalog = parseProviderCatalog(providerCatalogData)

func parseProviderCatalog(raw string) []ProviderField {
	var fields []ProviderField
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 11)
		if len(parts) != 9 && len(parts) != 11 {
			panic(fmt.Sprintf("invalid provider catalog row: %q", line))
		}
		required, err := strconv.ParseBool(parts[4])
		if err != nil {
			panic(fmt.Sprintf("invalid provider catalog required flag: %q", line))
		}
		secret, err := strconv.ParseBool(parts[5])
		if err != nil {
			panic(fmt.Sprintf("invalid provider catalog secret flag: %q", line))
		}
		group, condition := "", ""
		if len(parts) == 11 {
			group, condition = parts[9], parts[10]
		}
		if err := validateProviderCondition(group, condition); err != nil {
			panic(fmt.Sprintf("invalid provider catalog condition: %q: %v", line, err))
		}
		fields = append(fields, ProviderField{
			Provider: parts[0], DisplayName: parts[1], Field: parts[2],
			Type: parts[3], Required: required, Secret: secret,
			Validation: parts[6], Default: parts[7], Description: parts[8],
			Group: group, Condition: condition,
		})
	}
	return fields
}

func validateProviderCondition(group, condition string) error {
	if condition == "" {
		return nil
	}
	if strings.HasPrefix(condition, "choice:") {
		if group == "" || strings.TrimPrefix(condition, "choice:") == "" {
			return fmt.Errorf("choice conditions need a group and value")
		}
		return nil
	}
	if condition == "at-least-one" {
		if group == "" {
			return fmt.Errorf("at-least-one conditions need a group")
		}
		return nil
	}
	if strings.HasPrefix(condition, "required-if:") {
		expression := strings.TrimPrefix(condition, "required-if:")
		if !strings.Contains(expression, "=") {
			return fmt.Errorf("required-if conditions need group=value")
		}
		return nil
	}
	return fmt.Errorf("unsupported condition %q", condition)
}

// ProviderCatalog returns a copy of the complete guided installer schema.
func ProviderCatalog() []ProviderField {
	result := make([]ProviderField, len(providerCatalog))
	copy(result, providerCatalog)
	return result
}
