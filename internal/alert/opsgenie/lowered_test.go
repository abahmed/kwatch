package opsgenie

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// The update that lowers a page (the previous version serves again)
// lowers the open alert's priority: Opsgenie's priority action, P3.
func TestOpsgenieLoweringUpdateLowersPriority(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	m := providertest.Update()
	m.Route.Severity = "warning"
	m.PagingOnly = true
	if err := c.SendIncident(context.Background(), m); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	for _, req := range rec.Requests() {
		if req.Path == "/v2/alerts/kwatch-dev-p-42/priority" {
			if got, _ := req.JSON(t)["priority"].(string); got != "P3" {
				t.Fatalf("priority = %q, want P3", got)
			}
			return
		}
	}
	t.Fatal("no priority update for the open alert")
}
