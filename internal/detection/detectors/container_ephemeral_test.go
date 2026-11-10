package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A debug container's failures belong to the person debugging, who
// watches it: they raise nothing.
func TestEphemeralContainerRaisesNothing(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	id := newID(kube.KindContainer, "ns", "p/debugger")
	put(model, id, retryNow, map[string]inventory.Value{
		kube.AttrEphemeral:   inventory.Bool(true),
		kube.AttrState:       inventory.Text("waiting"),
		kube.AttrStateReason: inventory.Text("ImagePullBackOff"),
	})
	e, _ := model.Entity(id)

	found := Container{}.Detect(testDetectorContext(model, retryNow), e)

	assert.Empty(t, found)
}
