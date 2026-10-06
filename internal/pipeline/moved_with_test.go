package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestMovedWithPolicyMovesItsNamespace(t *testing.T) {
	policy := inventory.Observation{
		Entity: inventory.CoreID(kube.KindNetworkPolicy, "shop", "deny")}

	assert.Equal(t, []inventory.EntityID{
		inventory.CoreID(kube.KindNamespace, "", "shop")}, movedWith(policy))
}

func TestMovedWithLeavesOtherObjectsAlone(t *testing.T) {
	pod := inventory.Observation{
		Entity: inventory.CoreID(kube.KindPod, "shop", "api-0")}

	assert.Empty(t, movedWith(pod))
}
