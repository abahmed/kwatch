package catalog

import (
	"sort"
	"testing"
)

func TestNamesAreSortedAndIndependent(t *testing.T) {
	names := Names()
	if !sort.StringsAreSorted(names) {
		t.Fatal("provider names are not sorted")
	}
	if len(names) == 0 {
		t.Fatal("provider catalog is empty")
	}
	names[0] = "changed"
	if Names()[0] == "changed" {
		t.Fatal("provider names leaked mutable state")
	}
}
