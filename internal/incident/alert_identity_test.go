package incident

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Two clusters without a name paging one account must not share an
// alert key for the same failure: one would silence and resolve the
// other's alert.
func TestAlertIdentityKeepsUnnamedClustersApart(t *testing.T) {
	root := entity(kube.KindDeployment, "web")
	key := func(identity string) string {
		m := NewManager(Config{IDNonce: testNonce}, nil)
		m.SetAlertIdentity(identity)
		return m.freeAlertKey(&Incident{ID: "a", Root: root})
	}

	east, west := key("uid-east"), key("uid-west")

	if east == west {
		t.Fatalf("both clusters got %q", east)
	}
	if east != key("uid-east") {
		t.Fatal("the key must be the same on every start of one cluster")
	}
	if !strings.HasPrefix(east, "alert-") || len(east) != len(west) {
		t.Fatalf("key format changed: %q", east)
	}
	if key("") != alertKey(root, "") {
		t.Fatal("without an identity the key must stay as it was")
	}
}

// An incident announced before the identity was known keeps its key, so
// its open alert is still updated and resolved after the upgrade.
func TestAlertIdentityLeavesAnnouncedKeysAlone(t *testing.T) {
	root := entity(kube.KindDeployment, "web")
	old := alertKey(root, "")
	record := Record{ID: "inc-1", Root: root, State: Open, AlertKey: old}
	record.Announced = at(0)
	m := NewManager(Config{IDNonce: testNonce}, nil)
	m.SetAlertIdentity("uid-east")

	m.Restore([]Record{record}, at(0))

	for _, p := range m.Incidents() {
		if p.AlertKey != old {
			t.Fatalf("restored key = %q, want %q", p.AlertKey, old)
		}
	}
	if len(m.Incidents()) != 1 {
		t.Fatalf("restored %d incidents, want 1", len(m.Incidents()))
	}
}
