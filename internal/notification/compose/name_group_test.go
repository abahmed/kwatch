package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func namedIDs(kind inventory.Kind, names ...string) []inventory.EntityID {
	out := make([]inventory.EntityID, 0, len(names))
	for _, name := range names {
		out = append(out, inventory.CoreID(kind, "staging", name))
	}
	return out
}

// A mixed list says each kind once, never once per name.
func TestNameListSaysEachKindOnceInAMixedList(t *testing.T) {
	home := inventory.CoreID(kube.KindNode, "", "n1")
	ids := append(namedIDs(kube.KindIngress, "a", "b", "c", "d", "e"),
		namedIDs(kube.KindService, "x", "y", "z")...)

	assert.Equal(t, "ingresses a, b, c and two others, and three "+
		"services in staging", nameList(home, ids))
}

func TestNameListKeepsSingleNamesOfMixedKinds(t *testing.T) {
	home := inventory.CoreID(kube.KindNode, "", "n1")
	ids := append(namedIDs(kube.KindIngress, "a"),
		namedIDs(kube.KindService, "x")...)

	assert.Equal(t, "ingress a and service x in staging",
		nameList(home, ids))
}

func TestNameListGroupsSameKindNamesSplitByAnotherKind(t *testing.T) {
	home := inventory.CoreID(kube.KindNode, "", "n1")
	ids := []inventory.EntityID{
		inventory.CoreID(kube.KindIngress, "staging", "a"),
		inventory.CoreID(kube.KindService, "staging", "x"),
		inventory.CoreID(kube.KindIngress, "staging", "b"),
	}

	assert.Equal(t, "ingresses a and b, and service x in staging",
		nameList(home, ids))
}
