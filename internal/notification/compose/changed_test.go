package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

func changeAt(
	id inventory.EntityID, actor string, min int, field string,
) inventory.Change {
	return inventory.Change{Entity: id, Actor: actor, At: at(min, 0),
		Fields: []inventory.FieldChange{{Path: field}}}
}

// A failure with no cause lists the latest changes next to it, newest
// first, as facts.
func TestNoCauseNamesWhatChangedLately(t *testing.T) {
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "app-config")
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	d := announce(unclearIncident(false))
	d.Facts.Changes = []inventory.Change{
		changeAt(api, "", 12, "spec.template.spec.containers[0].image"),
		changeAt(cm, "bob", 10, "data.LOG"),
	}

	text := notification.Text(Writer{}.Write(d, at(15, 0)))

	want := "In the last 30 minutes in shop: the api rollout started at " +
		"14:12; bob changed config map app-config at 14:10."
	if !strings.Contains(text, want) {
		t.Fatalf("want %q in:\n%s", want, text)
	}
}

// Changes are only said when nothing explains the failure.
func TestChangesAreSilentWhenACauseIsKnown(t *testing.T) {
	cm := inventory.CoreID(kube.KindConfigMap, "shop", "app-config")
	p := unclearIncident(true)
	d := announce(p)
	d.Facts.Changes = []inventory.Change{changeAt(cm, "bob", 10, "data.LOG")}

	text := notification.Text(Writer{}.Write(d, at(15, 0)))

	if strings.Contains(text, "In the last 30 minutes") {
		t.Fatalf("a root with its own finding names no changes:\n%s", text)
	}
}
