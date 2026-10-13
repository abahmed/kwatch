package replay_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
)

// A Secret's attributes and field changes carry key names and digests,
// never values. They are hidden anyway, even words that look like reason
// codes, so a Secret leaves a recording as pseudonyms and numbers only.
func TestSanitizerHidesEverythingTextualAboutSecrets(t *testing.T) {
	at := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	secret := inventory.CoreID("secret", "shop", "Pull")
	obs := []inventory.Observation{
		{Kind: inventory.Observed, Source: "kubernetes", At: at,
			Entity: secret, Attributes: map[string]inventory.Value{
				"secret.type": inventory.Text("Opaque"),
				"keys":        inventory.Text("Token"),
				"digest":      inventory.Text("Hunter2"),
				"ready":       inventory.Bool(true),
				"count":       inventory.Number(2),
			}},
		{Kind: inventory.Changed, Source: "kubernetes", At: at,
			Entity: secret, Change: inventory.Change{Entity: secret, At: at,
				Fields: []inventory.FieldChange{{Path: "data.Token",
					Before: "Hunter2", After: "Hunter3"}}}},
	}
	s := mustSanitizer(t, replay.SanitizeOptions{Salt: "salt"})
	for _, o := range obs {
		data, err := json.Marshal(s.Observation(o))
		if err != nil {
			t.Fatal(err)
		}
		for _, leak := range []string{"Opaque", "Token", "Hunter2",
			"Hunter3", "Pull", "shop"} {
			if strings.Contains(string(data), leak) {
				t.Errorf("%q leaked in %s", leak, data)
			}
		}
	}
	clean := s.Observation(obs[0])
	if v, _ := clean.Attributes["count"].AsNumber(); v != 2 {
		t.Error("numbers must stay")
	}
	if b, _ := clean.Attributes["ready"].AsBool(); !b {
		t.Error("booleans must stay")
	}
	before := s.Observation(obs[1]).Change.Fields[0]
	if before.Before == before.After {
		t.Error("a changed secret key must still read as changed")
	}
}
