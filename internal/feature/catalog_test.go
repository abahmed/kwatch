package feature

import "testing"

func TestCatalogLookupAndValidation(t *testing.T) {
	if err := ValidateCatalog(); err != nil {
		t.Fatalf("validate catalog: %v", err)
	}
	definitions := Catalog()
	if len(definitions) == 0 {
		t.Fatal("feature catalog is empty")
	}
	first := definitions[0]
	if got, ok := Lookup(first.ID); !ok || got.ID != first.ID {
		t.Fatalf("lookup(%q) = %#v, %v", first.ID, got, ok)
	}
	if _, ok := Lookup(ID("missing.feature")); ok {
		t.Fatal("unknown feature was found")
	}
	for index := range definitions {
		if len(definitions[index].Dependencies) == 0 {
			continue
		}
		original := definitions[index].Dependencies[0]
		definitions[index].Dependencies[0] = ID("changed")
		copyOfCatalog := Catalog()
		if copyOfCatalog[index].Dependencies[0] != original {
			t.Fatal("catalog dependency slice leaked mutable state")
		}
		break
	}
}
