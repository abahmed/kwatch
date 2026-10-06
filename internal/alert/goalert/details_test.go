package goalert

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestGoalertDetailsAreBounded(t *testing.T) {
	c, rec := newTestGoalert(t)
	m := providertest.Announce()
	m.Output = []string{strings.Repeat("x", 3*safetext.DetailsLimit)}
	if err := c.SendIncident(context.Background(), m); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	details := decodePayload(t, rec.Last(t).Body).Details
	if len(details) > safetext.DetailsLimit {
		t.Fatalf("details are %d bytes", len(details))
	}
	if !strings.HasPrefix(details, m.Note) {
		t.Fatal("the note must survive the cut")
	}
}
