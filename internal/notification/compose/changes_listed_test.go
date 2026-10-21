package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A node's taints follow its conditions: a node that went NotReady and
// came back has them set and unset by the node controller. That is
// nobody's change, so it is never listed as something that changed
// before the failure.
func TestListedChangesLeaveOutTheNodeControllersOwnTaints(t *testing.T) {
	node := inventory.CoreID(kube.KindNode, "", "n2")
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "app-config")
	f := caseFacts{
		p: badRollout(),
		changes: []inventory.Change{
			{Entity: node, At: at(1, 0), Fields: []inventory.FieldChange{{
				Path:   "taints",
				Before: "node.kubernetes.io/unreachable:NoExecute"}}},
			{Entity: cm, At: at(2, 0), Actor: "bob",
				Fields: []inventory.FieldChange{{Path: "data.A",
					Before: "1", After: "2"}}},
		},
	}

	got := changesBeforeSentences(f)

	assert.Len(t, got, 1)
	assert.NotContains(t, got[0].text, "taints")
	assert.Contains(t, got[0].text, "bob changed config map app-config")
}

func TestOnlyTheNodeControllersTaintsLeaveNothingToList(t *testing.T) {
	node := inventory.CoreID(kube.KindNode, "", "n2")
	f := caseFacts{
		p: badRollout(),
		changes: []inventory.Change{
			{Entity: node, At: at(1, 0), Fields: []inventory.FieldChange{{
				Path: "taints", Before: "x:NoExecute"}}},
		},
	}

	assert.Empty(t, changesBeforeSentences(f))
	f.p.Cause = nil
	f.p.Root = inventory.CoreID(kube.KindDeployment, "shop", "api")
	assert.Empty(t, recentChangeSentence(f))
}
