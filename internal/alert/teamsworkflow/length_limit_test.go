package teamsworkflow

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A message far over TeamsWorkflow's size limit is cut to the limit, so the
// service accepts it instead of rejecting it for good.
func TestTeamsWorkflowCutsOverlongIncident(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewTeamsWorkflow(
		map[string]interface{}{"webhook": rec.URL()}, "",
		rec.Dependencies())
	if err := c.SendIncident(
		context.Background(), providertest.Oversized(300000),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	body := rec.Last(t)
	providertest.AssertWithin(t, "text", cardText(t, body.Body),
		teamsWorkflowTextLimit)
}
