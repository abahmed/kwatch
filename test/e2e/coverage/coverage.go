package coverage

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Entry struct {
	ID          string `yaml:"id"`
	Family      string `yaml:"family"`
	Lifecycle   string `yaml:"lifecycle"`
	Status      string `yaml:"status"`
	Test        string `yaml:"test,omitempty"`
	Environment string `yaml:"environment,omitempty"`
}

type Catalog struct {
	Entries []Entry `yaml:"entries"`
}

func Load(path string) (Catalog, error) {
	payload, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return Catalog{}, err
	}
	var catalog Catalog
	if err := yaml.Unmarshal(payload, &catalog); err != nil {
		return Catalog{}, fmt.Errorf("decode coverage: %w", err)
	}
	if err := Validate(catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func Validate(catalog Catalog) error {
	seen := make(map[string]struct{}, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		if entry.ID == "" || entry.Family == "" || entry.Lifecycle == "" {
			return fmt.Errorf("coverage entries require id, family, lifecycle")
		}
		if entry.Environment != "" && entry.Environment != "kind" &&
			entry.Environment != "kind-extended" {
			return fmt.Errorf(
				"coverage entry %q has invalid environment %q",
				entry.ID, entry.Environment,
			)
		}
		if entry.Status == "covered" && entry.Test == "" {
			return fmt.Errorf("covered entry %q requires test", entry.ID)
		}
		if _, ok := seen[entry.ID]; ok {
			return fmt.Errorf("duplicate coverage entry %q", entry.ID)
		}
		seen[entry.ID] = struct{}{}
		switch entry.Status {
		case "covered", "planned", "skipped-optional", "unsupported-in-kind":
		default:
			return fmt.Errorf(
				"coverage entry %q has invalid status %q",
				entry.ID, entry.Status,
			)
		}
	}
	if len(catalog.Entries) == 0 {
		return fmt.Errorf("coverage catalog is empty")
	}
	return nil
}
