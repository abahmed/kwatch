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

func TestValidateTiers(t *testing.T) {
	defs := []Definition{
		{ID: "a", Lifecycle: Runtime},
		{ID: "b", Lifecycle: Runtime, Dependencies: []ID{"a"}},
	}
	tests := []struct {
		name    string
		tiers   map[ID]Tier
		wantErr bool
	}{
		{"community on community",
			map[ID]Tier{"a": Community, "b": Community}, false},
		{"pro depends on community", map[ID]Tier{"a": Community}, false},
		{"community depends on pro", map[ID]Tier{"b": Community}, true},
		{"unknown id", map[ID]Tier{"zzz": Community}, true},
		{"invalid tier", map[ID]Tier{"a": Tier("gold")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(defs, tt.tiers)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
