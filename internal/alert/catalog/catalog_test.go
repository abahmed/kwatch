package catalog

import (
	"bufio"
	"os"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/config"
	providercatalog "github.com/abahmed/kwatch/internal/provider/catalog"
)

func TestProviderCatalogMatchesFactories(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderCatalogHasMetadataForEveryCanonicalProvider(t *testing.T) {
	metadata := make(map[string]bool)
	for _, field := range config.ProviderCatalog() {
		metadata[field.Provider] = true
	}

	for _, name := range providercatalog.Names() {
		// incident.io is an intentional alias of incidentio and shares its
		// configuration schema and factory.
		if name == "incident.io" {
			continue
		}
		if !metadata[name] {
			t.Errorf("provider %q has no configuration metadata", name)
		}
	}
	for name := range metadata {
		if !config.IsKnownProvider(name) {
			t.Errorf("metadata has unknown provider %q", name)
		}
	}
}

func TestProviderCatalogManifestCoversIdentities(t *testing.T) {
	file, err := os.Open("../../../deploy/provider-catalog.tsv")
	if err != nil {
		t.Fatalf("open generated provider catalog: %v", err)
	}
	defer file.Close()

	manifest := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.SplitN(line, "|", 2)
		if len(fields) == 2 {
			manifest[fields[0]] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read generated provider catalog: %v", err)
	}
	for _, name := range providercatalog.Names() {
		canonical := name
		if target, ok := providercatalog.Aliases()[name]; ok {
			canonical = target
		}
		if !manifest[canonical] {
			t.Errorf("provider %q is missing from generated catalog", name)
		}
	}
}

func TestProviderCatalogAliasesAreExplicit(t *testing.T) {
	aliases := providercatalog.Aliases()
	if aliases["incident.io"] != "incidentio" {
		t.Fatalf("incident.io alias is not explicit: %v", aliases)
	}
	for alias, canonical := range aliases {
		if alias == canonical || !config.IsKnownProvider(alias) ||
			!config.IsKnownProvider(canonical) {
			t.Fatalf("invalid provider alias %q -> %q", alias, canonical)
		}
	}
}
