package announce

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

func TestCollectorNamedSpellsCustomKindsWithoutTouchingTheInput(
	t *testing.T,
) {
	kind := inventory.Kind("autoscalinglistener")
	c := New(Env{KindNames: func(incident.Incident) map[inventory.Kind]string {
		return map[inventory.Kind]string{kind: "AutoscalingListener"}
	}})
	in := []incident.Decision{{Incident: incident.Incident{ID: "a"}}}

	out := c.named(in)

	assert.Equal(t, "AutoscalingListener", out[0].Facts.KindNames[kind])
	assert.Nil(t, in[0].Facts.KindNames, "the held decision is unchanged")
	assert.Nil(t, New(Env{}).named(in)[0].Facts.KindNames)
}
