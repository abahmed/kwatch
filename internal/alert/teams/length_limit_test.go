package teams

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A message far over Teams's size limit is cut to the limit, so the
// service accepts it instead of rejecting it for good.
func TestTeamsCutsOverlongIncident(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewTeams(map[string]interface{}{"webhook": rec.URL()},
		"", rec.Dependencies())
	if err := c.SendIncident(
		context.Background(), providertest.Oversized(300000),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	body := rec.Last(t)
	providertest.AssertWithin(t, "text", body.JSON(t)["text"].(string),
		teamsTextLimit)
}

func TestTeamsCutsOverlongMessage(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewTeams(map[string]interface{}{"webhook": rec.URL()},
		"", rec.Dependencies())
	if err := c.SendMessage(context.Background(),
		strings.Repeat("x", 300000)); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	providertest.AssertWithin(t, "text",
		rec.Last(t).JSON(t)["text"].(string), teamsTextLimit)
}
