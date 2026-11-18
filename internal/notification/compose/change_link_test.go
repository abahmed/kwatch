package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// A change that records the end of a link says what ended, between the
// two objects, instead of a field path.
func TestNoCauseNamesAnEndedLinkInWords(t *testing.T) {
	svc := inventory.CoreID(kube.KindService, "shop", "payments")
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "app-config")
	d := announce(unclearIncident(false))
	d.Facts.Changes = []inventory.Change{
		{Entity: svc, Actor: "bob", At: at(12, 0),
			Fields: []inventory.FieldChange{
				inventory.LinkRemoved(inventory.RoutesTo, api)}},
		{Entity: api, At: at(10, 0), Fields: []inventory.FieldChange{
			inventory.LinkRemoved(inventory.References, cm)}},
	}

	text := notification.Text(Writer{}.Write(d, at(15, 0)))

	want := "In the last 30 minutes in shop: service payments stopped " +
		"routing to api at 14:12 UTC by bob; api no longer uses config " +
		"map app-config at 14:10 UTC."
	if !strings.Contains(text, want) {
		t.Fatalf("want %q in:\n%s", want, text)
	}
}
